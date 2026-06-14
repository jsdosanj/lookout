package collect

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jsdosanj/lookout/internal/collect/exec"
	"github.com/jsdosanj/lookout/internal/collect/inventory"
	"github.com/jsdosanj/lookout/internal/collect/posture"
	"github.com/jsdosanj/lookout/internal/collect/software"
	"github.com/jsdosanj/lookout/internal/collector"
)

// runnerFor builds a runner allow-listing the three reference collectors' tools
// (without touching the process-wide registry the wiring helper uses).
func runnerFor() *exec.Runner {
	return exec.NewRunner(dedupStrings(
		inventory.DefaultAllow(), software.DefaultAllow(), posture.DefaultAllow(),
	))
}

// TestReferenceCollectorsSmoke runs the three real collectors on the host and
// asserts each returns a JSON-serializable record with its canonical SchemaID.
// This is the live verify for C1/C2/C4 on whatever OS the test runs on.
func TestReferenceCollectorsSmoke(t *testing.T) {
	runner := runnerFor()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cases := []struct {
		c        collector.Collector
		schemaID string
	}{
		{inventory.New(runner), inventory.SchemaID},
		{software.New(runner), software.SchemaID},
		{posture.New(runner), posture.SchemaID},
	}
	for _, tc := range cases {
		rec, err := tc.c.Collect(ctx)
		if err != nil {
			t.Errorf("%s: collect error: %v", tc.schemaID, err)
			continue
		}
		if rec.SchemaID != tc.schemaID {
			t.Errorf("%s: record SchemaID = %q", tc.schemaID, rec.SchemaID)
		}
		if _, err := json.Marshal(rec.Payload); err != nil {
			t.Errorf("%s: payload not JSON-serializable: %v", tc.schemaID, err)
		}
	}
}

// TestInventoryHasHostFacts asserts the inventory collector populates the
// always-available cross-platform fields from the standard library.
func TestInventoryHasHostFacts(t *testing.T) {
	rec, err := inventory.New(runnerFor()).Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	inv, ok := rec.Payload.(inventory.Inventory)
	if !ok {
		t.Fatalf("payload type = %T", rec.Payload)
	}
	if inv.Hostname == "" {
		t.Error("hostname should be populated from os.Hostname")
	}
	if inv.OS.Arch == "" || inv.OS.Name == "" {
		t.Error("OS name/arch should be populated from runtime")
	}
	if inv.CPU.Cores <= 0 {
		t.Error("CPU cores should be > 0")
	}
}

// TestPostureTriState asserts posture facts are never the empty string — a
// readable fact is on/off, an unreadable one is explicitly "unknown" so an
// auditor can't mistake absence for a pass.
func TestPostureTriState(t *testing.T) {
	rec, err := posture.New(runnerFor()).Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	p := rec.Payload.(posture.Posture)
	if p.Firewall.Enabled == "" || p.DiskEncryption.State == "" {
		t.Fatal("posture facts must be tri-state, never empty")
	}
	if len(p.CISChecks) == 0 {
		t.Fatal("posture should derive CIS checks from facts")
	}
}
