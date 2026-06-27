package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jsdosanj/lookout/internal/collect"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "lookout.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func report(host string, cpu float64) *collect.Report {
	return &collect.Report{
		Host:  collect.Host{Hostname: host},
		Specs: collect.Specs{CPUPercent: cpu, MemTotalMB: 100, MemUsedMB: 40},
	}
}

func TestStoreSaveGetList(t *testing.T) {
	st := openTemp(t)
	if err := st.Save(report("web-01", 10)); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(report("db-01", 20)); err != nil {
		t.Fatal(err)
	}
	if got := st.List(); len(got) != 2 || got[0].ID != "db-01" || got[1].ID != "web-01" {
		t.Fatalf("List sorted by ID: got %+v", got)
	}
	srv, ok := st.Get("web-01")
	if !ok || srv.LastReport.Specs.CPUPercent != 10 {
		t.Fatalf("Get web-01: ok=%v srv=%+v", ok, srv)
	}
	if len(srv.History) != 1 {
		t.Fatalf("want 1 history sample, got %d", len(srv.History))
	}
}

// TestStoreDurability is the core SQLite guarantee: state survives a restart.
func TestStoreDurability(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lookout.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := st.Save(report("web-01", float64(i))); err != nil {
			t.Fatal(err)
		}
	}
	_ = st.Close()

	st2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	srv, ok := st2.Get("web-01")
	if !ok {
		t.Fatal("server did not survive reopen")
	}
	if len(srv.History) != 3 {
		t.Fatalf("history did not survive reopen: got %d samples", len(srv.History))
	}
}

// TestStoreRetention verifies the retention sweep prunes samples older than the
// window on each Save, while keeping recent ones.
func TestStoreRetention(t *testing.T) {
	st := openTemp(t)
	st.SetRetention(time.Hour)

	// Inject an old sample directly, then a Save (which prunes) must drop it.
	old := time.Now().UTC().Add(-2 * time.Hour)
	if _, err := st.db.Exec(`INSERT INTO samples (server_id, at, cpu, mem, disk) VALUES (?,?,?,?,?)`,
		"web-01", old.UnixNano(), 1, 1, 1); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(report("web-01", 5)); err != nil {
		t.Fatal(err)
	}
	srv, _ := st.Get("web-01")
	if len(srv.History) != 1 {
		t.Fatalf("retention should have pruned the old sample, leaving 1; got %d", len(srv.History))
	}
}

func TestStoreHealthConfigPersist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lookout.db")
	st, _ := Open(path)
	// Fresh store returns defaults.
	if st.HealthConfig().Defaults.DiskCritPct != DefaultThresholds().DiskCritPct {
		t.Fatal("fresh store should expose default thresholds")
	}
	cfg := DefaultHealthConfig()
	cfg.Hosts = map[string]Thresholds{"web-01": {DiskCritPct: 75}}
	if err := st.SetHealthConfig(cfg); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()

	st2, _ := Open(path)
	defer st2.Close()
	if got := st2.HealthConfig().For("web-01").DiskCritPct; got != 75 {
		t.Fatalf("health config did not persist: got %v", got)
	}
}

func TestStoreAcks(t *testing.T) {
	st := openTemp(t)
	until := time.Now().UTC().Add(time.Hour).Round(0)
	if err := st.SaveAck("rule-1", "web-01", until); err != nil {
		t.Fatal(err)
	}
	rows, err := st.Acks()
	if err != nil || len(rows) != 1 {
		t.Fatalf("Acks: err=%v rows=%v", err, rows)
	}
	if rows[0].RuleID != "rule-1" || rows[0].Server != "web-01" || !rows[0].Until.Equal(until) {
		t.Fatalf("ack round-trip mismatch: %+v (want until %v)", rows[0], until)
	}
	// Upsert replaces, not duplicates.
	if err := st.SaveAck("rule-1", "web-01", until.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if rows, _ := st.Acks(); len(rows) != 1 {
		t.Fatalf("upsert should keep one row, got %d", len(rows))
	}
	if err := st.DeleteAck("rule-1", "web-01"); err != nil {
		t.Fatal(err)
	}
	if rows, _ := st.Acks(); len(rows) != 0 {
		t.Fatalf("delete should remove the ack, got %d", len(rows))
	}
}

// TestStoreLegacyMigration covers importing the pre-SQLite JSON data file.
func TestStoreLegacyMigration(t *testing.T) {
	st := openTemp(t)
	legacy := `[{"id":"old-01","last_seen":"2026-01-01T00:00:00Z",
		"last_report":{"host":{"hostname":"old-01"},"specs":{"cpu_percent":42}},
		"history":[{"at":"2026-01-01T00:00:00Z","cpu":42,"mem":10,"disk":5}]}]`
	if err := st.ImportLegacyJSON([]byte(legacy)); err != nil {
		t.Fatal(err)
	}
	srv, ok := st.Get("old-01")
	if !ok || srv.LastReport.Specs.CPUPercent != 42 || len(srv.History) != 1 {
		t.Fatalf("legacy import mismatch: ok=%v srv=%+v", ok, srv)
	}
}
