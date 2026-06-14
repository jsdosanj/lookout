package spool

import (
	"testing"
	"time"

	"github.com/jsdosanj/lookout/internal/collector"
)

func env(nonce string, ts time.Time) collector.Envelope {
	return collector.Envelope{AgentID: "a1", TenantID: "t1", Nonce: nonce, Timestamp: ts, SchemaID: "lookout.inventory.v1"}
}

// TestAddDrainRemove proves the core replay-safe flow: an added envelope is
// drained intact and only disappears after an explicit Remove.
func TestAddDrainRemove(t *testing.T) {
	s, err := New(t.TempDir(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add(env("n1", time.Unix(100, 0))); err != nil {
		t.Fatal(err)
	}
	if s.Len() != 1 {
		t.Fatalf("len=%d want 1", s.Len())
	}

	got, err := s.Drain(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Envelope.Nonce != "n1" {
		t.Fatalf("drain mismatch: %+v", got)
	}

	// Replay-safety: not removed yet ⇒ still drainable.
	if s.Len() != 1 {
		t.Fatal("entry must remain until Remove (at-least-once delivery)")
	}
	if err := s.Remove(got[0].File); err != nil {
		t.Fatal(err)
	}
	if s.Len() != 0 {
		t.Fatal("entry should be gone after Remove")
	}
}

// TestOldestFirst proves Drain returns chronological order regardless of insert
// order (oldest-first replay).
func TestOldestFirst(t *testing.T) {
	s, _ := New(t.TempDir(), 1<<20)
	_, _ = s.Add(env("late", time.Unix(300, 0)))
	_, _ = s.Add(env("early", time.Unix(100, 0)))
	_, _ = s.Add(env("mid", time.Unix(200, 0)))

	got, _ := s.Drain(10)
	want := []string{"early", "mid", "late"}
	for i, w := range want {
		if got[i].Envelope.Nonce != w {
			t.Fatalf("order[%d]=%q want %q", i, got[i].Envelope.Nonce, w)
		}
	}
}

// TestCapEvictsOldest proves the size cap drops the oldest entries first.
func TestCapEvictsOldest(t *testing.T) {
	// Each entry JSON is well over 50 bytes; cap at ~120 bytes keeps ~1-2.
	s, _ := New(t.TempDir(), 200)
	for i := 0; i < 10; i++ {
		_, _ = s.Add(env(string(rune('a'+i)), time.Unix(int64(100+i), 0)))
	}
	got, _ := s.Drain(100)
	if len(got) == 0 || len(got) >= 10 {
		t.Fatalf("cap not enforced: kept %d entries", len(got))
	}
	// Whatever survived must be the NEWEST (largest timestamps).
	first := got[0].Envelope.Timestamp.Unix()
	if first <= 100 {
		t.Fatalf("oldest entry survived eviction (ts=%d)", first)
	}
}
