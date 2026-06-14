package alert

import (
	"testing"
	"time"
)

// capture is a test Channel that records every notification it receives.
type capture struct {
	id   string
	sent []Notification
}

func (c *capture) ID() string { return c.id }
func (c *capture) Send(n Notification) error {
	c.sent = append(c.sent, n)
	return nil
}

// newEngine builds an engine with one capture channel and the given rule.
func newEngine(r Rule) (*Engine, *capture) {
	c := &capture{id: "cap"}
	r.Channels = []string{"cap"}
	return NewEngine([]Rule{r}, []Channel{c}, nil), c
}

func TestSeverityOf(t *testing.T) {
	cases := map[string]Severity{
		"ok": SevNone, "": SevNone, "warning": SevWarning,
		"critical": SevCritical, "stale": SevStale,
	}
	for in, want := range cases {
		if got := SeverityOf(in); got != want {
			t.Errorf("SeverityOf(%q)=%v, want %v", in, got, want)
		}
	}
}

// Dedupe: an ongoing problem fires once, not on every report.
func TestDedupeFiresOncePerIncident(t *testing.T) {
	eng, cap := newEngine(Rule{ID: "r", Name: "r", Server: "*", MinSeverity: SevWarning, FlapWindow: 1})
	now := time.Now()
	for i := 0; i < 5; i++ {
		eng.Observe("h1", "warning", "disk 85%", now.Add(time.Duration(i)*time.Minute))
	}
	if len(cap.sent) != 1 {
		t.Fatalf("dedupe: want 1 notification, got %d", len(cap.sent))
	}
	if cap.sent[0].Severity != "warning" || cap.sent[0].Resolved {
		t.Errorf("unexpected first notification: %+v", cap.sent[0])
	}
}

// Escalation by severity: warning -> critical fires the change once.
func TestEscalationOnSeverityChange(t *testing.T) {
	eng, cap := newEngine(Rule{ID: "r", Name: "r", Server: "*", MinSeverity: SevWarning, FlapWindow: 1})
	now := time.Now()
	eng.Observe("h1", "warning", "disk 85%", now)
	eng.Observe("h1", "warning", "disk 85%", now.Add(time.Minute))
	eng.Observe("h1", "critical", "disk 95%", now.Add(2*time.Minute))
	if len(cap.sent) != 2 {
		t.Fatalf("want 2 notifications (warning then critical), got %d", len(cap.sent))
	}
	if cap.sent[1].Severity != "critical" {
		t.Errorf("second notification severity = %q, want critical", cap.sent[1].Severity)
	}
}

// Resolve: recovery fires exactly one resolved notification.
func TestResolveOnRecovery(t *testing.T) {
	eng, cap := newEngine(Rule{ID: "r", Name: "r", Server: "*", MinSeverity: SevWarning, FlapWindow: 1})
	now := time.Now()
	eng.Observe("h1", "critical", "disk 95%", now)
	eng.Observe("h1", "ok", "", now.Add(time.Minute))
	eng.Observe("h1", "ok", "", now.Add(2*time.Minute)) // stays healthy, no extra send
	if len(cap.sent) != 2 {
		t.Fatalf("want 2 notifications (fire + resolve), got %d", len(cap.sent))
	}
	if !cap.sent[1].Resolved {
		t.Errorf("second notification should be resolved: %+v", cap.sent[1])
	}
}

// Flap-damping: a value bouncing across the threshold within the window must not
// fire until the new state is confirmed for FlapWindow consecutive observations.
func TestFlapDampingSuppressesBounce(t *testing.T) {
	eng, cap := newEngine(Rule{ID: "r", Name: "r", Server: "*", MinSeverity: SevWarning, FlapWindow: 3})
	now := time.Now()
	// Bounce ok/warning/ok/warning — never 3 consecutive of the same — no fire.
	seq := []string{"warning", "ok", "warning", "ok", "warning", "ok"}
	for i, s := range seq {
		eng.Observe("h1", s, "flapping", now.Add(time.Duration(i)*time.Minute))
	}
	if len(cap.sent) != 0 {
		t.Fatalf("flapping should not fire, got %d notifications: %+v", len(cap.sent), cap.sent)
	}
	// Now hold warning for 3 in a row — should fire once.
	for i := 0; i < 3; i++ {
		eng.Observe("h1", "warning", "disk 85%", now.Add(time.Duration(10+i)*time.Minute))
	}
	if len(cap.sent) != 1 {
		t.Fatalf("confirmed warning: want 1 notification, got %d", len(cap.sent))
	}
}

// MinSeverity floor: a rule that fires only on critical must ignore warning.
func TestMinSeverityFloor(t *testing.T) {
	eng, cap := newEngine(Rule{ID: "r", Name: "r", Server: "*", MinSeverity: SevCritical, FlapWindow: 1})
	now := time.Now()
	eng.Observe("h1", "warning", "disk 85%", now)
	if len(cap.sent) != 0 {
		t.Fatalf("warning below critical floor should not fire, got %d", len(cap.sent))
	}
	eng.Observe("h1", "critical", "disk 95%", now.Add(time.Minute))
	if len(cap.sent) != 1 {
		t.Fatalf("critical should fire, got %d", len(cap.sent))
	}
}

// Repeat/escalation reminder: an unresolved incident re-notifies after RepeatEvery.
func TestRepeatReminder(t *testing.T) {
	eng, cap := newEngine(Rule{ID: "r", Name: "r", Server: "*", MinSeverity: SevWarning,
		FlapWindow: 1, RepeatEvery: 30 * time.Minute})
	now := time.Now()
	eng.Observe("h1", "critical", "disk 95%", now)                     // fire
	eng.Observe("h1", "critical", "disk 95%", now.Add(10*time.Minute)) // within cadence: deduped
	if len(cap.sent) != 1 {
		t.Fatalf("within cadence should dedupe, got %d", len(cap.sent))
	}
	eng.Observe("h1", "critical", "disk 95%", now.Add(31*time.Minute)) // cadence elapsed: remind
	if len(cap.sent) != 2 {
		t.Fatalf("after cadence should remind, got %d", len(cap.sent))
	}
	if !cap.sent[1].Repeat {
		t.Errorf("second notification should be a repeat reminder: %+v", cap.sent[1])
	}
}

// Server matching: a rule scoped to one server ignores others.
func TestServerScopedRule(t *testing.T) {
	eng, cap := newEngine(Rule{ID: "r", Name: "r", Server: "h1", MinSeverity: SevWarning, FlapWindow: 1})
	now := time.Now()
	eng.Observe("h2", "critical", "x", now) // different server: ignored
	eng.Observe("h1", "warning", "y", now)  // matched
	if len(cap.sent) != 1 || cap.sent[0].Server != "h1" {
		t.Fatalf("server-scoped rule: want 1 for h1, got %d %+v", len(cap.sent), cap.sent)
	}
}

// Ack stops the reminder cascade: an acknowledged incident does not re-notify
// even after RepeatEvery elapses, but a recovery still fires the resolve.
func TestAcknowledgeStopsReminders(t *testing.T) {
	eng, cap := newEngine(Rule{ID: "r", Name: "r", Server: "*", MinSeverity: SevWarning,
		FlapWindow: 1, RepeatEvery: 30 * time.Minute})
	now := time.Now()
	eng.Observe("h1", "critical", "disk 95%", now) // fire (1)
	if !eng.Acknowledge("r", "h1", time.Time{}) {
		t.Fatal("acknowledge should find the open incident")
	}
	// Two repeat windows elapse — with the ack, neither reminds.
	eng.Observe("h1", "critical", "disk 95%", now.Add(31*time.Minute))
	eng.Observe("h1", "critical", "disk 95%", now.Add(62*time.Minute))
	if len(cap.sent) != 1 {
		t.Fatalf("ack should silence reminders, got %d notifications", len(cap.sent))
	}
	// Recovery still resolves (ack stops nagging, not the all-clear).
	eng.Observe("h1", "ok", "", now.Add(70*time.Minute))
	if len(cap.sent) != 2 || !cap.sent[1].Resolved {
		t.Fatalf("recovery after ack should resolve, got %+v", cap.sent)
	}
}

// Ack does not mask a worsening problem: a severity escalation clears the ack and
// re-alerts.
func TestAcknowledgeClearedByEscalation(t *testing.T) {
	eng, cap := newEngine(Rule{ID: "r", Name: "r", Server: "*", MinSeverity: SevWarning,
		FlapWindow: 1, RepeatEvery: 30 * time.Minute})
	now := time.Now()
	eng.Observe("h1", "warning", "disk 85%", now) // fire (1)
	eng.Acknowledge("r", "h1", time.Time{})
	eng.Observe("h1", "critical", "disk 95%", now.Add(time.Minute)) // escalation re-alerts (2)
	if len(cap.sent) != 2 || cap.sent[1].Severity != "critical" {
		t.Fatalf("escalation must clear ack and re-alert, got %+v", cap.sent)
	}
}

// Snooze suppresses reminders until the snooze expires, then they resume.
func TestSnoozeExpires(t *testing.T) {
	eng, cap := newEngine(Rule{ID: "r", Name: "r", Server: "*", MinSeverity: SevWarning,
		FlapWindow: 1, RepeatEvery: 30 * time.Minute})
	now := time.Now()
	eng.Observe("h1", "critical", "disk 95%", now)                     // fire (1)
	eng.Acknowledge("r", "h1", now.Add(45*time.Minute))                // snooze 45m
	eng.Observe("h1", "critical", "disk 95%", now.Add(31*time.Minute)) // within snooze: silent
	if len(cap.sent) != 1 {
		t.Fatalf("within snooze should be silent, got %d", len(cap.sent))
	}
	eng.Observe("h1", "critical", "disk 95%", now.Add(50*time.Minute)) // snooze expired: remind (2)
	if len(cap.sent) != 2 || !cap.sent[1].Repeat {
		t.Fatalf("after snooze should remind, got %+v", cap.sent)
	}
}

func TestEnabled(t *testing.T) {
	var nilEng *Engine
	if nilEng.Enabled() {
		t.Error("nil engine must not be enabled")
	}
	if NewEngine(nil, nil, nil).Enabled() {
		t.Error("engine with no rules/channels must not be enabled")
	}
	eng, _ := newEngine(Rule{ID: "r", Name: "r", Server: "*", MinSeverity: SevWarning})
	if !eng.Enabled() {
		t.Error("engine with a rule and channel should be enabled")
	}
}
