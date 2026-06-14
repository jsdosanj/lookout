// Package inventory is collector C1: system inventory & hardware.
//
// Emits the canonical evidence type lookout.inventory.v1 consumed by Cairn
// (asset reconciliation), Sightline (asset evidence) and Ledger (hardware
// register). Cross-platform best-effort: every field degrades gracefully to a
// zero value rather than failing the whole record, so a locked-down host still
// reports what it can.
//
// All OS facts come from the standard library or allow-listed, argv-only
// commands via the shared exec.Runner — no shell, ever.
package inventory

import (
	"context"
	"os"
	"runtime"
	"time"

	"github.com/jsdosanj/lookout/internal/collect/exec"
	"github.com/jsdosanj/lookout/internal/collector"
)

// SchemaID is the canonical evidence type for this collector.
const SchemaID = "lookout.inventory.v1"

// Inventory is the lookout.inventory.v1 payload.
type Inventory struct {
	Hostname       string  `json:"hostname"`
	FQDN           string  `json:"fqdn,omitempty"`
	OS             OSInfo  `json:"os"`
	Serial         string  `json:"serial,omitempty"`
	Manufacturer   string  `json:"manufacturer,omitempty"`
	Model          string  `json:"model,omitempty"`
	CPU            CPUInfo `json:"cpu"`
	RAMBytes       uint64  `json:"ram_bytes,omitempty"`
	Disks          []Disk  `json:"disks,omitempty"`
	NICs           []NIC   `json:"nics,omitempty"`
	BootTime       string  `json:"boot_time,omitempty"` // RFC3339, best-effort
	UptimeSeconds  int64   `json:"uptime_seconds,omitempty"`
	Domain         string  `json:"domain,omitempty"`
	Virtualization string  `json:"virtualization,omitempty"`
}

// OSInfo describes the operating system.
type OSInfo struct {
	Name    string `json:"name"` // e.g. "darwin", "linux", "windows" (or distro)
	Version string `json:"version,omitempty"`
	Build   string `json:"build,omitempty"`
	Arch    string `json:"arch"`
}

// CPUInfo describes the processor.
type CPUInfo struct {
	Model string `json:"model,omitempty"`
	Cores int    `json:"cores"`
}

// Disk is one fixed disk / volume (bytes).
type Disk struct {
	Name       string `json:"name,omitempty"`
	TotalBytes uint64 `json:"total_bytes,omitempty"`
}

// NIC is one network interface.
type NIC struct {
	Name string `json:"name"`
	MAC  string `json:"mac,omitempty"`
	IP   string `json:"ip,omitempty"`
	Type string `json:"type,omitempty"` // ethernet, wifi, ... (best-effort)
}

// Collector is C1.
type Collector struct{ runner *exec.Runner }

// New builds the inventory collector with a safe-exec runner. The runner must
// allow-list the platform inventory tools (see DefaultAllow).
func New(runner *exec.Runner) *Collector { return &Collector{runner: runner} }

// DefaultAllow lists the binaries this collector may invoke, per OS. The agent
// passes these (merged across collectors) to exec.NewRunner.
func DefaultAllow() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"system_profiler", "sysctl", "sw_vers"}
	case "linux":
		return []string{"lscpu", "uname"}
	case "windows":
		return []string{"wmic"}
	}
	return nil
}

// Meta implements collector.Collector. Inventory needs no special capability
// beyond basic read; it carries a read.inventory cap so policy can still gate
// it per tenant if desired.
func (c *Collector) Meta() collector.Metadata {
	return collector.Metadata{
		ID:              "system_inventory",
		SchemaID:        SchemaID,
		RequiredCaps:    []collector.Capability{"read.inventory"},
		DefaultInterval: 6 * time.Hour,
	}
}

// Collect gathers the inventory snapshot. Cross-platform parts use the standard
// library; OS-specific hardware facts use osDetail (build-tagged).
func (c *Collector) Collect(ctx context.Context) (collector.Record, error) {
	inv := Inventory{
		OS: OSInfo{
			Name: runtime.GOOS,
			Arch: runtime.GOARCH,
		},
		CPU: CPUInfo{Cores: runtime.NumCPU()},
	}
	if h, err := os.Hostname(); err == nil {
		inv.Hostname = h
	}
	inv.NICs = networkInterfaces()

	// OS-specific enrichment (serial, model, ram, os version, boot time, ...).
	c.osDetail(ctx, &inv)

	return collector.Record{SchemaID: SchemaID, Payload: inv}, nil
}
