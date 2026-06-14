package alert

import (
	"path/filepath"
	"testing"
	"time"
)

func TestRuleStoreSeedsDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.json")
	rs, err := OpenRuleStore(path, []string{"webhook"})
	if err != nil {
		t.Fatal(err)
	}
	got := rs.Rules()
	if len(got) != 1 || got[0].ID != "fleet-default" || got[0].MinSeverity != SevWarning {
		t.Fatalf("seed default rule wrong: %+v", got)
	}
	// Re-open from the same file: the persisted rule (not a re-seed) loads back.
	rs2, err := OpenRuleStore(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs2.Rules()) != 1 {
		t.Fatalf("reopen: want 1 persisted rule, got %d", len(rs2.Rules()))
	}
}

func TestRuleStoreUpsertDeletePersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.json")
	rs, _ := OpenRuleStore(path, []string{"webhook"})

	saved, err := rs.Upsert(Rule{Name: "db only", Server: "db1", MinSeverity: SevCritical,
		Channels: []string{"webhook"}, FlapWindow: 1, RepeatEvery: 10 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID == "" {
		t.Fatal("upsert should assign an ID to a new rule")
	}
	// Update in place by ID.
	saved.MinSeverity = SevWarning
	if _, err := rs.Upsert(saved); err != nil {
		t.Fatal(err)
	}
	if n := len(rs.Rules()); n != 2 { // default + db rule
		t.Fatalf("after update want 2 rules, got %d", n)
	}

	// Persisted round-trip preserves the edited severity and repeat cadence.
	rs2, _ := OpenRuleStore(path, nil)
	var found *Rule
	for _, r := range rs2.Rules() {
		if r.ID == saved.ID {
			rr := r
			found = &rr
		}
	}
	if found == nil || found.MinSeverity != SevWarning || found.RepeatEvery != 10*time.Minute {
		t.Fatalf("persisted rule not faithful: %+v", found)
	}

	if err := rs.Delete(saved.ID); err != nil {
		t.Fatal(err)
	}
	if n := len(rs.Rules()); n != 1 {
		t.Fatalf("after delete want 1 rule, got %d", n)
	}
}
