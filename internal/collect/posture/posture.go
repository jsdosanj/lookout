// Package posture is collector C4: security posture / configuration.
//
// Emits lookout.posture.v1, consumed by Sightline (control evidence across 22+
// frameworks), Bastion (CMMC / 800-171 control answers) and Ward (config
// drift / hardening). It reports firewall, disk encryption, screen lock,
// antivirus presence, MFA hints, and a small set of CIS-style local checks.
//
// Cross-platform best-effort: each fact degrades to Unknown rather than failing
// the record, so a partially-readable host still produces usable evidence. All
// OS facts come from allow-listed argv-only commands (no shell) or file reads.
package posture

import (
	"context"
	"runtime"
	"time"

	"github.com/jsdosanj/lookout/internal/collect/exec"
	"github.com/jsdosanj/lookout/internal/collector"
)

// SchemaID is the canonical evidence type for this collector.
const SchemaID = "lookout.posture.v1"

// State is a tri-state for a posture fact: we distinguish "off" from "couldn't
// determine" because a compliance auditor must not read Unknown as a pass.
type State string

const (
	StateOn      State = "on"
	StateOff     State = "off"
	StateUnknown State = "unknown"
)

// Posture is the lookout.posture.v1 payload.
type Posture struct {
	Firewall       Firewall   `json:"firewall"`
	DiskEncryption Encryption `json:"disk_encryption"`
	ScreenLock     ScreenLock `json:"screen_lock"`
	Antivirus      Antivirus  `json:"antivirus"`
	MFAHints       MFAHints   `json:"mfa_hints"`
	CISChecks      []CISCheck `json:"cis_checks,omitempty"`
}

// Firewall is the host firewall state.
type Firewall struct {
	Enabled State `json:"enabled"`
}

// Encryption is the disk-encryption state and method.
type Encryption struct {
	State  State  `json:"state"`
	Method string `json:"method,omitempty"` // FileVault, BitLocker, LUKS
}

// ScreenLock is the auto-lock / screensaver-password state.
type ScreenLock struct {
	Enabled        State `json:"enabled"`
	TimeoutSeconds int   `json:"timeout_seconds,omitempty"`
}

// Antivirus is endpoint-protection presence + real-time protection.
type Antivirus struct {
	Present State  `json:"present"`
	Enabled State  `json:"enabled"`
	Product string `json:"product,omitempty"`
}

// MFAHints reports local indicators that MFA may be present (best-effort —
// authoritative MFA state comes from the IdP connectors, not the endpoint).
type MFAHints struct {
	ProviderPresent State  `json:"provider_present"`
	Detail          string `json:"detail,omitempty"`
}

// CISCheck is one CIS-style local hardening check result. The check catalog is
// intentionally small and fixed in wave0 (no arbitrary on-host policy eval).
type CISCheck struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Result   string `json:"result"` // pass | fail | na
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
}

// Collector is C4.
type Collector struct{ runner *exec.Runner }

// New builds the posture collector with a safe-exec runner.
func New(runner *exec.Runner) *Collector { return &Collector{runner: runner} }

// DefaultAllow lists the binaries this collector may invoke, per OS.
func DefaultAllow() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"fdesetup", "defaults", "socketfilterfw", "spctl", "system_profiler"}
	case "linux":
		return []string{"ufw", "systemctl"}
	case "windows":
		return []string{"netsh", "manage-bde", "powershell"}
	}
	return nil
}

// Meta implements collector.Collector. Posture reads security configuration, so
// it carries a read.posture capability the operator policy can gate.
func (c *Collector) Meta() collector.Metadata {
	return collector.Metadata{
		ID:              "posture",
		SchemaID:        SchemaID,
		RequiredCaps:    []collector.Capability{"read.posture"},
		DefaultInterval: time.Hour,
	}
}

// Collect gathers the posture snapshot via the OS-specific gatherer, then
// derives the CIS-style checks from the collected facts so the check results
// stay consistent with the raw fields.
func (c *Collector) Collect(ctx context.Context) (collector.Record, error) {
	p := Posture{
		Firewall:       Firewall{Enabled: StateUnknown},
		DiskEncryption: Encryption{State: StateUnknown},
		ScreenLock:     ScreenLock{Enabled: StateUnknown},
		Antivirus:      Antivirus{Present: StateUnknown, Enabled: StateUnknown},
		MFAHints:       MFAHints{ProviderPresent: StateUnknown},
	}
	c.gather(ctx, &p)
	p.CISChecks = deriveCISChecks(&p)
	return collector.Record{SchemaID: SchemaID, Payload: p}, nil
}

// deriveCISChecks maps the collected posture facts onto a small fixed set of
// CIS-style checks. Keeping the catalog derived (not independently re-queried)
// guarantees the check result and the raw field never disagree.
func deriveCISChecks(p *Posture) []CISCheck {
	stateResult := func(s State, want State) string {
		switch s {
		case want:
			return "pass"
		case StateUnknown:
			return "na"
		default:
			return "fail"
		}
	}
	return []CISCheck{
		{
			ID: "LK-CIS-1.1", Title: "Disk encryption enabled",
			Result:   stateResult(p.DiskEncryption.State, StateOn),
			Expected: string(StateOn), Actual: string(p.DiskEncryption.State),
		},
		{
			ID: "LK-CIS-2.1", Title: "Host firewall enabled",
			Result:   stateResult(p.Firewall.Enabled, StateOn),
			Expected: string(StateOn), Actual: string(p.Firewall.Enabled),
		},
		{
			ID: "LK-CIS-3.1", Title: "Screen lock / auto-lock enabled",
			Result:   stateResult(p.ScreenLock.Enabled, StateOn),
			Expected: string(StateOn), Actual: string(p.ScreenLock.Enabled),
		},
		{
			ID: "LK-CIS-4.1", Title: "Endpoint protection present",
			Result:   stateResult(p.Antivirus.Present, StateOn),
			Expected: string(StateOn), Actual: string(p.Antivirus.Present),
		},
	}
}
