// Package scheduler runs registered collectors on jittered intervals under hard
// resource budgets, capability gating, and per-collector circuit breakers.
//
// This is the layer that directly answers the osquery "runaway query melts the
// endpoint / drains the battery" complaints (plan §3, §4): every collector run
// is bounded by a wall-clock timeout, gated on operator-granted capabilities,
// and tripped offline by a circuit breaker after repeated failures/overruns so
// one misbehaving collector can never pin the host.
//
// What's enforced here in wave0:
//   - Capability gating: a collector runs only if ALL its RequiredCaps are
//     granted by policy.
//   - Wall-clock budget: each run gets a context with a timeout; overrun ==
//     failure (the collector must honor ctx; the budget is the backstop).
//   - Circuit breaker: N consecutive failures opens the breaker for a cooldown;
//     while open the collector is skipped.
//   - Jittered intervals: each collector ticks at its interval ± jitter so a
//     fleet doesn't synchronize a thundering herd at the ingest endpoint.
//
// CPU%/RSS niceness budgets are declared in Budget and partially enforced
// (timeout). Hard CPU/RSS capping needs OS-specific cgroup/job-object plumbing
// — marked TODO(wave0).
//
// Standard library only.
package scheduler

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/jsdosanj/lookout/internal/collector"
)

// Budget is the per-collector resource envelope. Timeout is enforced now; the
// CPU/RSS fields are declared and reported, with hard enforcement deferred.
type Budget struct {
	Timeout time.Duration // wall-clock cap per run (enforced)
	// TODO(wave0): MaxCPUPercent / MaxRSSBytes hard enforcement via
	// cgroups (linux) / job objects (windows) / setpriority (darwin).
	MaxCPUPercent int
	MaxRSSBytes   int64
}

// DefaultBudget is a conservative envelope for passive collectors.
var DefaultBudget = Budget{Timeout: 30 * time.Second, MaxCPUPercent: 20, MaxRSSBytes: 256 << 20}

// Policy is the operator-controlled run policy for a collector, normally
// derived from the signed agent policy (GET /v1/policy). Absent policy ⇒ the
// collector's registry defaults are used and NO capabilities are granted (fail
// closed: a capability-requiring collector won't run until explicitly granted).
type Policy struct {
	Enabled     bool
	Interval    time.Duration // overrides Metadata.DefaultInterval when > 0
	Budget      Budget        // overrides DefaultBudget when Timeout > 0
	GrantedCaps map[collector.Capability]bool
}

// Sink receives a successful collector Record for shipping. Returning an error
// does not fail the collector run (the record may be spooled by the shipper);
// it's only logged via the OnEvent hook.
type Sink func(ctx context.Context, c collector.Collector, rec collector.Record) error

// Event is an observability callback payload for run lifecycle (feeds C12
// health telemetry: last-run map, error counts, breaker state).
type Event struct {
	CollectorID string
	Started     time.Time
	Duration    time.Duration
	Err         error  // nil on success
	Skipped     string // non-empty reason when the run was skipped (gated/breaker)
	BreakerOpen bool
}

// Config wires a Scheduler.
type Config struct {
	Registry   *collector.Registry
	Policies   map[string]Policy // by collector id; absent ⇒ defaults + no caps
	Sink       Sink
	OnEvent    func(Event)
	JitterFrac float64 // fraction of interval to jitter, e.g. 0.1 == ±10%
	// Breaker tuning.
	BreakerThreshold int           // consecutive failures to open the breaker
	BreakerCooldown  time.Duration // how long the breaker stays open
}

// Scheduler ticks collectors per policy under budgets and breakers.
type Scheduler struct {
	cfg      Config
	mu       sync.Mutex
	breakers map[string]*breaker
	rng      *rand.Rand
}

// New builds a Scheduler with sane defaults for unset Config fields.
func New(cfg Config) *Scheduler {
	if cfg.JitterFrac <= 0 {
		cfg.JitterFrac = 0.1
	}
	if cfg.BreakerThreshold <= 0 {
		cfg.BreakerThreshold = 3
	}
	if cfg.BreakerCooldown <= 0 {
		cfg.BreakerCooldown = 10 * time.Minute
	}
	return &Scheduler{
		cfg:      cfg,
		breakers: make(map[string]*breaker),
		rng:      rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Run drives all registered collectors until ctx is cancelled. Each collector
// gets its own goroutine + ticker so a slow collector never blocks others.
func (s *Scheduler) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for _, c := range s.cfg.Registry.All() {
		c := c
		pol := s.policyFor(c)
		if !pol.Enabled {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.loop(ctx, c, pol)
		}()
	}
	wg.Wait()
}

// RunOnce executes one collector immediately (backs the control-plane "run-now"
// command). It applies the same gating/budget/breaker rules as the loop.
func (s *Scheduler) RunOnce(ctx context.Context, id string) {
	c, ok := s.cfg.Registry.Get(id)
	if !ok {
		return
	}
	s.tick(ctx, c, s.policyFor(c))
}

func (s *Scheduler) loop(ctx context.Context, c collector.Collector, pol Policy) {
	interval := pol.Interval
	if interval <= 0 {
		interval = c.Meta().DefaultInterval
	}
	if interval <= 0 {
		interval = time.Hour
	}
	// Initial jittered delay spreads first-runs across the fleet.
	timer := time.NewTimer(s.jitter(interval))
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			s.tick(ctx, c, pol)
			timer.Reset(s.jitter(interval))
		}
	}
}

// tick runs one collection attempt with gating, budget, and breaker logic.
func (s *Scheduler) tick(ctx context.Context, c collector.Collector, pol Policy) {
	meta := c.Meta()
	start := time.Now()

	// Circuit breaker check.
	if s.breaker(meta.ID).open() {
		s.emit(Event{CollectorID: meta.ID, Started: start, Skipped: "circuit breaker open", BreakerOpen: true})
		return
	}

	// Capability gating: fail closed if any required cap isn't granted.
	if reason := missingCap(meta.RequiredCaps, pol.GrantedCaps); reason != "" {
		s.emit(Event{CollectorID: meta.ID, Started: start, Skipped: "missing capability: " + reason})
		return
	}

	budget := pol.Budget
	if budget.Timeout <= 0 {
		budget = DefaultBudget
	}
	runCtx, cancel := context.WithTimeout(ctx, budget.Timeout)
	defer cancel()

	rec, err := safeCollect(runCtx, c)
	dur := time.Since(start)

	br := s.breaker(meta.ID)
	if err != nil {
		opened := br.fail(s.cfg.BreakerThreshold, s.cfg.BreakerCooldown)
		s.emit(Event{CollectorID: meta.ID, Started: start, Duration: dur, Err: err, BreakerOpen: opened})
		return
	}
	br.success()

	if s.cfg.Sink != nil {
		_ = s.cfg.Sink(ctx, c, rec) // ship errors are spooled by the shipper, not fatal here
	}
	s.emit(Event{CollectorID: meta.ID, Started: start, Duration: dur})
}

// safeCollect runs Collect and converts a panic into an error so one buggy
// collector can never crash the agent.
func safeCollect(ctx context.Context, c collector.Collector) (rec collector.Record, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = recoveredPanic(r)
		}
	}()
	return c.Collect(ctx)
}

func (s *Scheduler) emit(e Event) {
	if s.cfg.OnEvent != nil {
		s.cfg.OnEvent(e)
	}
}

func (s *Scheduler) policyFor(c collector.Collector) Policy {
	if p, ok := s.cfg.Policies[c.Meta().ID]; ok {
		return p
	}
	// No policy ⇒ enabled with defaults but NO granted caps (fail closed for
	// capability-requiring collectors; cap-free collectors still run).
	return Policy{Enabled: true}
}

// jitter returns d ± JitterFrac*d.
func (s *Scheduler) jitter(d time.Duration) time.Duration {
	if d <= 0 {
		return time.Second
	}
	s.mu.Lock()
	frac := (s.rng.Float64()*2 - 1) * s.cfg.JitterFrac // [-f, +f]
	s.mu.Unlock()
	out := time.Duration(float64(d) * (1 + frac))
	if out <= 0 {
		out = d
	}
	return out
}

func (s *Scheduler) breaker(id string) *breaker {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.breakers[id]
	if !ok {
		b = &breaker{}
		s.breakers[id] = b
	}
	return b
}

// missingCap returns the first required capability not present in granted, or ""
// if all are granted.
func missingCap(required []collector.Capability, granted map[collector.Capability]bool) string {
	for _, cap := range required {
		if !granted[cap] {
			return string(cap)
		}
	}
	return ""
}
