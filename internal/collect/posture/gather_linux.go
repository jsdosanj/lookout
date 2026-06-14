//go:build linux

package posture

import (
	"context"
	"os"
	"strings"
)

// gather fills Linux posture facts. Prefers file reads (no exec); falls back to
// allow-listed argv-only commands for firewall state.
func (c *Collector) gather(ctx context.Context, p *Posture) {
	// Disk encryption: LUKS. /proc/crypt or the presence of a dm-crypt mapping
	// is the dependency-free signal; reading /sys is best-effort.
	p.DiskEncryption.Method = "LUKS"
	if luksPresent() {
		p.DiskEncryption.State = StateOn
	} else {
		p.DiskEncryption.State = StateOff
	}

	// Firewall: try ufw (Ubuntu) then firewalld via systemctl.
	if c.runner != nil {
		if c.runner.Allowed("ufw") {
			if out, err := c.runner.Run(ctx, "ufw", "status"); err == nil {
				if strings.Contains(out, "Status: active") {
					p.Firewall.Enabled = StateOn
				} else if strings.Contains(out, "Status: inactive") {
					p.Firewall.Enabled = StateOff
				}
			}
		}
		if p.Firewall.Enabled == StateUnknown && c.runner.Allowed("systemctl") {
			if out, err := c.runner.Run(ctx, "systemctl", "is-active", "firewalld"); err == nil {
				if strings.TrimSpace(out) == "active" {
					p.Firewall.Enabled = StateOn
				}
			}
		}
	}

	// Screen lock and AV on Linux desktops vary wildly by DE/distro; leave
	// Unknown in wave0 rather than guess. TODO(wave0): GNOME/KDE idle-lock
	// dconf reads, ClamAV/auditd presence as AV signal.
}

// luksPresent reports whether any active dm-crypt (LUKS) mapping exists, read
// from /proc/mounts + /sys without exec.
func luksPresent() bool {
	// Active LUKS volumes appear as /dev/mapper/* dm-crypt devices; the kernel
	// lists crypt targets under /sys/block/dm-*/dm/. A cheap, dependency-free
	// heuristic: any /dev/mapper entry whose backing uuid starts with CRYPT-LUKS.
	b, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) > 0 && strings.HasPrefix(f[0], "/dev/mapper/") {
			// Confirm it's a crypt target, not LVM, via the dm uuid.
			name := strings.TrimPrefix(f[0], "/dev/mapper/")
			if dmIsCrypt(name) {
				return true
			}
		}
	}
	return false
}

// dmIsCrypt checks the device-mapper uuid for a CRYPT- prefix.
func dmIsCrypt(name string) bool {
	// /sys/block/<dm>/dm/uuid holds e.g. "CRYPT-LUKS2-...". We can't easily map
	// the mapper name to the dm-N node without scanning; do a best-effort scan.
	entries, err := os.ReadDir("/sys/block")
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "dm-") {
			continue
		}
		nameFile := "/sys/block/" + e.Name() + "/dm/name"
		uuidFile := "/sys/block/" + e.Name() + "/dm/uuid"
		nb, _ := os.ReadFile(nameFile)
		if strings.TrimSpace(string(nb)) != name {
			continue
		}
		ub, _ := os.ReadFile(uuidFile)
		return strings.HasPrefix(strings.TrimSpace(string(ub)), "CRYPT-")
	}
	return false
}
