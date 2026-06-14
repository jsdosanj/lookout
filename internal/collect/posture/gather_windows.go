//go:build windows

package posture

import (
	"context"
	"strings"
)

// gather fills Windows posture facts via allow-listed argv-only commands.
func (c *Collector) gather(ctx context.Context, p *Posture) {
	if c.runner == nil {
		return
	}

	// Firewall: `netsh advfirewall show allprofiles state`. If every profile
	// reports ON we call it on; any OFF ⇒ off.
	if out, err := c.runner.Run(ctx, "netsh", "advfirewall", "show", "allprofiles", "state"); err == nil {
		p.Firewall.Enabled = parseNetshFirewall(out)
	}

	// Disk encryption: BitLocker via manage-bde status on the system drive.
	p.DiskEncryption.Method = "BitLocker"
	if out, err := c.runner.Run(ctx, "manage-bde", "-status", "C:"); err == nil {
		lo := strings.ToLower(out)
		switch {
		case strings.Contains(lo, "protection on"):
			p.DiskEncryption.State = StateOn
		case strings.Contains(lo, "protection off"):
			p.DiskEncryption.State = StateOff
		}
	}

	// Antivirus + screen lock: Defender/AV product state and the
	// machine-inactivity-limit policy are best read via PowerShell CIM cmdlets.
	// TODO(wave0): query Get-MpComputerStatus (Defender RTP) and the
	// InactivityTimeoutSecs policy key, argv-only via powershell -NoProfile
	// -Command with a fixed script string (no interpolation of untrusted data).
	// Left Unknown in wave0 to keep the dependency-free baseline honest.
}

// parseNetshFirewall reduces the multi-profile "State ON/OFF" output to a single
// State: on only if no profile is OFF and at least one is ON.
func parseNetshFirewall(out string) State {
	sawOn, sawOff := false, false
	for _, line := range strings.Split(out, "\n") {
		l := strings.ToLower(strings.TrimSpace(line))
		if !strings.HasPrefix(l, "state") {
			continue
		}
		switch {
		case strings.HasSuffix(l, "on"):
			sawOn = true
		case strings.HasSuffix(l, "off"):
			sawOff = true
		}
	}
	switch {
	case sawOff:
		return StateOff
	case sawOn:
		return StateOn
	default:
		return StateUnknown
	}
}
