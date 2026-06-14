package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jsdosanj/lookout/internal/collector"
)

// fakeCollector is a configurable test collector.
type fakeCollector struct {
	meta collector.Metadata
	mu   sync.Mutex
	runs int
	err  error
	slow time.Duration
}

func (f *fakeCollector) Meta() collector.Metadata { return f.meta }
func (f *fakeCollector) Collect(ctx context.Context) (collector.Record, error) {
	f.mu.Lock()
	f.runs++
	f.mu.Unlock()
	if f.slow > 0 {
		select {
		case <-time.After(f.slow):
		case <-ctx.Done():
			return collector.Record{}, ctx.Err()
		}
	}
	if f.err != nil {
		return collector.Record{}, f.err
	}
	return collector.Record{SchemaID: f.meta.SchemaID, Payload: f.runs}, nil
}

// regWith registers c into the process-wide registry and returns it. Test
// collectors use unique IDs so registration never collides across tests.
func regWith(c collector.Collector) *collector.Registry {
	collector.Register(c)
	return collector.Default()
}

func newSched(c collector.Collector, pol Policy, onEvent func(Event)) *Scheduler {
	return New(Config{
		Registry: regWith(c),
		Policies: map[string]Policy{c.Meta().ID: pol},
		OnEvent:  onEvent,
	})
}

// TestCapabilityGating proves a collector whose required cap is NOT granted is
// skipped with a "missing capability" reason (fail-closed).
func TestCapabilityGating(t *testing.T) {
	fc := &fakeCollector{meta: collector.Metadata{
		ID: "gate_denied", SchemaID: "s", RequiredCaps: []collector.Capability{"exec.trivy"},
	}}
	var skipped string
	s := newSched(fc, Policy{Enabled: true, Budget: Budget{Timeout: time.Second}}, func(e Event) {
		if e.Skipped != "" {
			skipped = e.Skipped
		}
	})
	s.RunOnce(context.Background(), "gate_denied")
	if fc.runs != 0 {
		t.Fatal("collector ran without its required capability")
	}
	if skipped == "" {
		t.Fatal("expected a skip event with a missing-capability reason")
	}
}

// TestCapabilityGranted proves the same collector runs once the cap is granted.
func TestCapabilityGranted(t *testing.T) {
	fc := &fakeCollector{meta: collector.Metadata{
		ID: "gate_granted", SchemaID: "s", RequiredCaps: []collector.Capability{"exec.trivy"},
	}}
	s := newSched(fc, Policy{
		Enabled:     true,
		Budget:      Budget{Timeout: time.Second},
		GrantedCaps: map[collector.Capability]bool{"exec.trivy": true},
	}, nil)
	s.RunOnce(context.Background(), "gate_granted")
	if fc.runs != 1 {
		t.Fatalf("granted collector should have run once, ran %d", fc.runs)
	}
}

// TestCircuitBreaker proves repeated failures trip the breaker so the collector
// stops being run.
func TestCircuitBreaker(t *testing.T) {
	fc := &fakeCollector{
		meta: collector.Metadata{ID: "flaky", SchemaID: "s"},
		err:  errors.New("boom"),
	}
	s := New(Config{
		Registry:         regWith(fc),
		Policies:         map[string]Policy{"flaky": {Enabled: true, Budget: Budget{Timeout: time.Second}}},
		BreakerThreshold: 3,
		BreakerCooldown:  time.Hour,
	})
	// 3 failures open the breaker; further ticks are skipped.
	for i := 0; i < 6; i++ {
		s.RunOnce(context.Background(), "flaky")
	}
	if fc.runs != 3 {
		t.Fatalf("breaker should cap runs at threshold=3, got %d", fc.runs)
	}
}

// TestTimeoutBudget proves a collector that overruns its wall-clock budget is
// cancelled and reported as an error.
func TestTimeoutBudget(t *testing.T) {
	fc := &fakeCollector{
		meta: collector.Metadata{ID: "slow", SchemaID: "s"},
		slow: 2 * time.Second,
	}
	var gotErr error
	s := New(Config{
		Registry: regWith(fc),
		Policies: map[string]Policy{"slow": {Enabled: true, Budget: Budget{Timeout: 50 * time.Millisecond}}},
		OnEvent:  func(e Event) { gotErr = e.Err },
	})
	start := time.Now()
	s.RunOnce(context.Background(), "slow")
	if time.Since(start) > time.Second {
		t.Fatal("run was not cancelled by the budget timeout")
	}
	if gotErr == nil {
		t.Fatal("expected a timeout error event")
	}
}
