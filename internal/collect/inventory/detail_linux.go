//go:build linux

package inventory

import (
	"context"
	"os"
	"strconv"
	"strings"
)

// osDetail enriches the inventory with Linux facts. Prefers reading sysfs/procfs
// files (no exec) and falls back to allow-listed argv-only commands.
func (c *Collector) osDetail(ctx context.Context, inv *Inventory) {
	// Distro name/version from /etc/os-release (pure file read).
	if b, err := os.ReadFile("/etc/os-release"); err == nil {
		name, ver := parseOSRelease(string(b))
		if name != "" {
			inv.OS.Name = name
		}
		inv.OS.Version = ver
	}

	// Kernel build from uname (file-free via /proc).
	if b, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		inv.OS.Build = strings.TrimSpace(string(b))
	}

	// RAM from /proc/meminfo (MemTotal is in kB).
	if b, err := os.ReadFile("/proc/meminfo"); err == nil {
		inv.RAMBytes = parseMemTotalKB(string(b)) * 1024
	}

	// CPU model from /proc/cpuinfo.
	if b, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		inv.CPU.Model = parseCPUModel(string(b))
	}

	// Uptime from /proc/uptime (seconds, float).
	if b, err := os.ReadFile("/proc/uptime"); err == nil {
		if f := strings.Fields(string(b)); len(f) > 0 {
			if secs, e := strconv.ParseFloat(f[0], 64); e == nil {
				inv.UptimeSeconds = int64(secs)
			}
		}
	}

	// DMI product/serial (often root-only; best-effort, no failure if denied).
	inv.Model = strings.TrimSpace(readFile("/sys/class/dmi/id/product_name"))
	inv.Manufacturer = strings.TrimSpace(readFile("/sys/class/dmi/id/sys_vendor"))
	inv.Serial = strings.TrimSpace(readFile("/sys/class/dmi/id/product_serial"))
}

func readFile(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return string(b)
}

// parseOSRelease pulls NAME and VERSION_ID from os-release content.
func parseOSRelease(s string) (name, version string) {
	for _, line := range strings.Split(s, "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = strings.Trim(strings.TrimSpace(v), `"`)
		switch strings.TrimSpace(k) {
		case "NAME":
			name = v
		case "VERSION_ID":
			version = v
		}
	}
	return name, version
}

// parseMemTotalKB returns the MemTotal value (kB) from /proc/meminfo.
func parseMemTotalKB(s string) uint64 {
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			f := strings.Fields(line)
			if len(f) >= 2 {
				if n, err := strconv.ParseUint(f[1], 10, 64); err == nil {
					return n
				}
			}
		}
	}
	return 0
}

// parseCPUModel returns the first "model name" from /proc/cpuinfo.
func parseCPUModel(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if k, v, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(k) == "model name" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
