//go:build darwin

package posture

import (
	"context"
	"strconv"
	"strings"
)

// gather fills macOS posture facts via allow-listed argv-only commands.
func (c *Collector) gather(ctx context.Context, p *Posture) {
	if c.runner == nil {
		return
	}

	// Disk encryption: FileVault.
	if out, err := c.runner.Run(ctx, "fdesetup", "status"); err == nil {
		p.DiskEncryption.Method = "FileVault"
		if strings.Contains(out, "FileVault is On") {
			p.DiskEncryption.State = StateOn
		} else if strings.Contains(out, "FileVault is Off") {
			p.DiskEncryption.State = StateOff
		}
	}

	// Application firewall (socketfilterfw).
	if out, err := c.runner.Run(ctx, "socketfilterfw", "--getglobalstate"); err == nil {
		switch {
		case strings.Contains(out, "enabled"):
			p.Firewall.Enabled = StateOn
		case strings.Contains(out, "disabled"):
			p.Firewall.Enabled = StateOff
		}
	}

	// Screen lock: askForPassword + grace delay from the screensaver domain.
	if out, err := c.runner.Run(ctx, "defaults", "read", "com.apple.screensaver", "askForPassword"); err == nil {
		if strings.TrimSpace(out) == "1" {
			p.ScreenLock.Enabled = StateOn
		} else {
			p.ScreenLock.Enabled = StateOff
		}
	}
	if out, err := c.runner.Run(ctx, "defaults", "read", "com.apple.screensaver", "askForPasswordDelay"); err == nil {
		if n, e := strconv.Atoi(strings.TrimSpace(out)); e == nil {
			p.ScreenLock.TimeoutSeconds = n
		}
	}

	// Gatekeeper as an endpoint-protection signal (macOS lacks a single AV API).
	if out, err := c.runner.Run(ctx, "spctl", "--status"); err == nil {
		if strings.Contains(out, "assessments enabled") {
			p.Antivirus.Present = StateOn
			p.Antivirus.Enabled = StateOn
			p.Antivirus.Product = "Gatekeeper"
		} else {
			p.Antivirus.Present = StateOff
		}
	}

	// MFA hint: presence of a smartcard/SmartCard pairing is a weak signal; we
	// leave this Unknown by default — authoritative MFA comes from IdP
	// connectors, not the endpoint (plan §9 / C4 mfa_hints).
}
