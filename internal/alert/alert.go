// Package alert is Lookout's built-in alerting engine. It turns the existing
// plain-English health state (ok / warning / critical / stale) into actionable
// notifications, with the production-grade behaviours operators expect:
//
//   - a real rule model (which servers, which minimum severity, which channels),
//   - dedupe so one ongoing problem fires once, not on every report,
//   - flap-damping so a value bouncing across a threshold doesn't spam,
//   - repeat/escalation so an unacknowledged incident re-notifies on a cadence,
//   - resolve notifications when a server recovers.
//
// The engine is transport-agnostic: it decides WHAT to send and WHEN, then hands
// a Notification to one or more Channels (see channel.go). External I/O lives
// behind the Channel boundary so the decision logic is pure and unit-tested.
package alert

import (
	"sort"
	"sync"
	"time"
)

// Severity is the actionable level of an alert, derived from health status.
// It is ordered: higher is worse.
type Severity int

const (
	SevNone     Severity = iota // ok — nothing to alert on
	SevWarning                  // warning
	SevCritical                 // critical
	SevStale                    // agent stopped reporting (treated as most severe)
)

func (s Severity) String() string {
	switch s {
	case SevWarning:
		return "warning"
	case SevCritical:
		return "critical"
	case SevStale:
		return "stale"
	default:
		return "ok"
	}
}

// SeverityOf maps a health status string to a Severity.
func SeverityOf(status string) Severity {
	switch status {
	case "warning":
		return SevWarning
	case "critical":
		return SevCritical
	case "stale":
		return SevStale
	default:
		return SevNone
	}
}

// Rule decides when a server's state is worth alerting on and where to send it.
type Rule struct {
	ID          string   // stable identifier
	Name        string   // human label
	Server      string   // "" or "*" matches any server; else an exact server ID
	MinSeverity Severity // fire only at or above this severity
	Channels    []string // channel IDs to deliver to (see Engine.channels)
	// FlapWindow is how many consecutive same-severity observations are required
	// before the engine acts on a change. 1 = act immediately (no damping).
	FlapWindow int
	// RepeatEvery re-notifies an unresolved incident on this cadence (0 = never
	// repeat). This is the escalation reminder for an unacknowledged problem.
	RepeatEvery time.Duration
}

func (r Rule) matches(server string) bool {
	return r.Server == "" || r.Server == "*" || r.Server == server
}

// Notification is one alert payload handed to channels. It is fully populated by
// the engine; channels only format/deliver it.
type Notification struct {
	RuleID   string    `json:"rule_id"`
	RuleName string    `json:"rule_name"`
	Server   string    `json:"server"`
	Severity string    `json:"severity"`
	Resolved bool      `json:"resolved"` // true when the server recovered
	Repeat   bool      `json:"repeat"`   // true when this is an escalation reminder
	Reason   string    `json:"reason"`   // plain-English cause, e.g. "disk /data is 94% full"
	At       time.Time `json:"at"`
}

// incident tracks one rule's active state for one server (dedupe + escalation).
type incident struct {
	severity   Severity  // current alerting severity (>= rule.MinSeverity)
	firstFired time.Time // when the incident first notified
	lastFired  time.Time // when we last notified (for RepeatEvery)
	reason     string
}

// flap tracks consecutive observations of one (rule,server) for flap-damping.
type flap struct {
	pending Severity // the severity we're waiting to confirm
	count   int      // consecutive observations of pending
}

// Channel delivers a Notification to one destination. Implementations isolate
// all external I/O (HTTP, SMTP); the engine never does network work itself.
type Channel interface {
	ID() string
	Send(Notification) error
}

// LogFunc records engine activity (deliveries, errors) for the dashboard's
// recent-activity surface and for diagnostics. May be nil.
type LogFunc func(n Notification, deliveredTo string, err error)

// Engine evaluates rules against health observations and drives channels. It is
// concurrency-safe: Observe may be called from many report handlers at once.
type Engine struct {
	mu        sync.Mutex
	rules     []Rule
	channels  map[string]Channel
	incidents map[string]*incident // key: ruleID|server
	flaps     map[string]*flap     // key: ruleID|server
	log       LogFunc
}

// NewEngine builds an engine from rules and channels. logf may be nil.
func NewEngine(rules []Rule, channels []Channel, logf LogFunc) *Engine {
	chans := make(map[string]Channel, len(channels))
	for _, c := range channels {
		chans[c.ID()] = c
	}
	return &Engine{
		rules:     rules,
		channels:  chans,
		incidents: map[string]*incident{},
		flaps:     map[string]*flap{},
		log:       logf,
	}
}

// Enabled reports whether any rule and channel are configured.
func (e *Engine) Enabled() bool {
	return e != nil && len(e.rules) > 0 && len(e.channels) > 0
}

func key(ruleID, server string) string { return ruleID + "|" + server }

// Observe feeds one health observation for a server into the engine and delivers
// any notifications the rules call for. now is the observation time (injected so
// tests are deterministic). reason is the plain-English cause for the worst
// signal (may be empty).
func (e *Engine) Observe(server, status, reason string, now time.Time) {
	if e == nil {
		return
	}
	sev := SeverityOf(status)
	e.mu.Lock()
	var toSend []sendJob
	for i := range e.rules {
		r := e.rules[i]
		if !r.matches(server) {
			continue
		}
		// Below this rule's floor, the rule treats the server as healthy.
		effective := sev
		if effective < r.MinSeverity {
			effective = SevNone
		}
		if jobs := e.evalRule(r, server, effective, reason, now); len(jobs) > 0 {
			toSend = append(toSend, jobs...)
		}
	}
	e.mu.Unlock()
	// Deliver outside the lock: channel Send may do (mocked) I/O.
	for _, j := range toSend {
		e.deliver(j.n, j.channels)
	}
}

type sendJob struct {
	n        Notification
	channels []string
}

// evalRule applies dedupe, flap-damping, and escalation for one rule+server.
// Caller holds e.mu. It returns the notifications to deliver (possibly none).
func (e *Engine) evalRule(r Rule, server string, sev Severity, reason string, now time.Time) []sendJob {
	k := key(r.ID, server)

	// Flap-damping: require FlapWindow consecutive observations of the same
	// severity before we treat it as the confirmed state.
	window := r.FlapWindow
	if window < 1 {
		window = 1
	}
	f := e.flaps[k]
	if f == nil || f.pending != sev {
		f = &flap{pending: sev, count: 1}
		e.flaps[k] = f
	} else if f.count < window {
		f.count++
	}
	if f.count < window {
		// Not yet confirmed; hold steady (no fire, no resolve).
		return nil
	}
	confirmed := sev

	inc := e.incidents[k]
	switch {
	case confirmed == SevNone:
		// Healthy. If an incident was open, resolve it (dedupe: once).
		if inc != nil {
			delete(e.incidents, k)
			return []sendJob{{
				n: Notification{RuleID: r.ID, RuleName: r.Name, Server: server,
					Severity: inc.severity.String(), Resolved: true, Reason: reason, At: now},
				channels: r.Channels,
			}}
		}
		return nil

	case inc == nil:
		// New incident: fire once.
		e.incidents[k] = &incident{severity: confirmed, firstFired: now, lastFired: now, reason: reason}
		return []sendJob{{
			n: Notification{RuleID: r.ID, RuleName: r.Name, Server: server,
				Severity: confirmed.String(), Reason: reason, At: now},
			channels: r.Channels,
		}}

	case confirmed != inc.severity:
		// Severity escalated or de-escalated while still unhealthy: fire the change.
		inc.severity = confirmed
		inc.lastFired = now
		inc.reason = reason
		return []sendJob{{
			n: Notification{RuleID: r.ID, RuleName: r.Name, Server: server,
				Severity: confirmed.String(), Reason: reason, At: now},
			channels: r.Channels,
		}}

	case r.RepeatEvery > 0 && now.Sub(inc.lastFired) >= r.RepeatEvery:
		// Same severity, still open, and the repeat cadence elapsed: re-notify.
		inc.lastFired = now
		return []sendJob{{
			n: Notification{RuleID: r.ID, RuleName: r.Name, Server: server,
				Severity: confirmed.String(), Repeat: true, Reason: inc.reason, At: now},
			channels: r.Channels,
		}}
	}
	// Same severity, within cadence: deduped (no send).
	return nil
}

// deliver sends one notification to each named channel, logging the outcome.
func (e *Engine) deliver(n Notification, channelIDs []string) {
	for _, id := range channelIDs {
		c := e.channels[id]
		if c == nil {
			continue
		}
		err := c.Send(n)
		if e.log != nil {
			e.log(n, id, err)
		}
	}
}

// Rules returns a copy of the configured rules (for the UI), sorted by name.
func (e *Engine) Rules() []Rule {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	out := append([]Rule(nil), e.rules...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ChannelIDs returns the configured channel IDs (for the UI), sorted.
func (e *Engine) ChannelIDs() []string {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]string, 0, len(e.channels))
	for id := range e.channels {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
