package alert

import (
	"sync"
	"testing"
	"time"
)

// fakeAckStore is an in-memory AckStore for testing persistence semantics.
type fakeAckStore struct {
	mu   sync.Mutex
	acks map[string]AckRecord // key: ruleID|server
}

func newFakeAckStore() *fakeAckStore { return &fakeAckStore{acks: map[string]AckRecord{}} }

func (f *fakeAckStore) SaveAck(ruleID, server string, until time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acks[ruleID+"|"+server] = AckRecord{RuleID: ruleID, Server: server, Until: until}
	return nil
}
func (f *fakeAckStore) DeleteAck(ruleID, server string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.acks, ruleID+"|"+server)
	return nil
}
func (f *fakeAckStore) LoadAcks() ([]AckRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]AckRecord, 0, len(f.acks))
	for _, r := range f.acks {
		out = append(out, r)
	}
	return out, nil
}
func (f *fakeAckStore) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.acks)
}

type capChan struct{ sent []Notification }

func (c *capChan) ID() string { return "cap" }
func (c *capChan) Send(n Notification) error {
	c.sent = append(c.sent, n)
	return nil
}

func critRule() Rule {
	return Rule{ID: "r", Name: "r", Server: "*", MinSeverity: SevWarning,
		FlapWindow: 1, RepeatEvery: 10 * time.Minute, Channels: []string{"cap"}}
}

// TestAckPersistedAndSurvivesRestart proves an acknowledgement is written through
// to the AckStore and, after a restart re-hydrates it, re-silences the reminder
// cascade once the incident re-forms.
func TestAckPersistedAndSurvivesRestart(t *testing.T) {
	store := newFakeAckStore()
	t0 := time.Now().UTC()

	// First engine: fire, then acknowledge (which must persist).
	cap1 := &capChan{}
	e1 := NewEngine([]Rule{critRule()}, []Channel{cap1}, nil)
	e1.SetAckStore(store)
	e1.Observe("h1", "critical", "disk full", t0)
	if !e1.Acknowledge("r", "h1", time.Time{}) {
		t.Fatal("acknowledge should find the open incident")
	}
	if store.count() != 1 {
		t.Fatalf("ack should be persisted, store has %d", store.count())
	}

	// Restart: a brand-new engine re-hydrates the ack from the store.
	cap2 := &capChan{}
	e2 := NewEngine([]Rule{critRule()}, []Channel{cap2}, nil)
	e2.SetAckStore(store)
	e2.Observe("h1", "critical", "disk full", t0.Add(time.Minute)) // incident re-forms (1 send)
	e2.Observe("h1", "critical", "disk full", t0.Add(20*time.Minute))
	if len(cap2.sent) != 1 {
		t.Fatalf("restored ack should suppress the repeat reminder; got %d sends", len(cap2.sent))
	}

	// Control: an identical engine with NO restored ack DOES repeat.
	cap3 := &capChan{}
	e3 := NewEngine([]Rule{critRule()}, []Channel{cap3}, nil)
	e3.Observe("h1", "critical", "disk full", t0.Add(time.Minute))
	e3.Observe("h1", "critical", "disk full", t0.Add(20*time.Minute))
	if len(cap3.sent) != 2 {
		t.Fatalf("control without ack should repeat; got %d sends", len(cap3.sent))
	}
}

// TestAckClearedOnResolve verifies a resolved incident drops its persisted ack so
// a future recurrence is not silently suppressed.
func TestAckClearedOnResolve(t *testing.T) {
	store := newFakeAckStore()
	t0 := time.Now().UTC()
	e := NewEngine([]Rule{critRule()}, []Channel{&capChan{}}, nil)
	e.SetAckStore(store)

	e.Observe("h1", "critical", "disk full", t0)
	e.Acknowledge("r", "h1", time.Time{})
	if store.count() != 1 {
		t.Fatalf("ack should be persisted, got %d", store.count())
	}
	e.Observe("h1", "ok", "", t0.Add(time.Minute)) // resolve
	if store.count() != 0 {
		t.Fatalf("resolve should clear the persisted ack, got %d", store.count())
	}
}
