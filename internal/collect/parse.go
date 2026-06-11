package collect

import (
	"strconv"
	"strings"
)

// All functions here are pure: raw text in, structured data out. No OS calls,
// so they compile and are unit-tested on every platform.

// parseOSRelease pulls the distro id and a human-readable version from the
// contents of /etc/os-release.
func parseOSRelease(content string) (id, version string) {
	vals := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		vals[k] = strings.Trim(v, `"`)
	}
	version = vals["PRETTY_NAME"]
	if version == "" {
		version = vals["VERSION"]
	}
	return vals["ID"], version
}

// parseMeminfo returns total and used memory (MB) from /proc/meminfo. Used is
// derived from MemAvailable, which reflects what the kernel can actually reclaim.
func parseMeminfo(content string) (totalMB, usedMB uint64) {
	var totalKB, availKB uint64
	for _, line := range strings.Split(content, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		val, _ := strconv.ParseUint(f[1], 10, 64) // kB
		switch f[0] {
		case "MemTotal:":
			totalKB = val
		case "MemAvailable:":
			availKB = val
		}
	}
	totalMB = totalKB / 1024
	if availKB > 0 && availKB <= totalKB {
		usedMB = (totalKB - availKB) / 1024
	}
	return totalMB, usedMB
}

// parseCPUInfo returns the CPU model and logical core count from /proc/cpuinfo,
// handling both x86 ("model name") and ARM ("Model"/"Hardware") layouts.
func parseCPUInfo(content string) (model string, cores int) {
	for _, line := range strings.Split(content, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "processor":
			cores++
		case "model name", "Model", "Hardware":
			if model == "" {
				model = strings.TrimSpace(v)
			}
		}
	}
	return model, cores
}

// parseUptime returns whole seconds from /proc/uptime ("12345.67 8910.11").
func parseUptime(content string) int64 {
	f := strings.Fields(content)
	if len(f) == 0 {
		return 0
	}
	secs, _ := strconv.ParseFloat(f[0], 64)
	return int64(secs)
}

// parseLoadAvg returns the 1/5/15-minute load from a string of space-separated
// floats (Linux /proc/loadavg, or macOS vm.loadavg with braces stripped).
func parseLoadAvg(content string) []float64 {
	f := strings.Fields(content)
	out := make([]float64, 0, 3)
	for i := 0; i < 3 && i < len(f); i++ {
		v, err := strconv.ParseFloat(f[i], 64)
		if err != nil {
			return nil
		}
		out = append(out, v)
	}
	if len(out) < 3 {
		return nil
	}
	return out
}

// parseDf parses POSIX `df -kP` output (1024-byte blocks) into disks, keeping
// only real, mounted filesystems.
func parseDf(output string) []Disk {
	var disks []Disk
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for i, line := range lines {
		if i == 0 {
			continue // header
		}
		f := strings.Fields(line)
		if len(f) < 6 {
			continue
		}
		totalKB, err1 := strconv.ParseUint(f[1], 10, 64)
		usedKB, err2 := strconv.ParseUint(f[2], 10, 64)
		mount := f[len(f)-1]
		if err1 != nil || err2 != nil || totalKB == 0 || !strings.HasPrefix(mount, "/") {
			continue
		}
		disks = append(disks, Disk{Mount: mount, FS: f[0], TotalMB: totalKB / 1024, UsedMB: usedKB / 1024})
	}
	return disks
}

// parseTabPackages parses "name<TAB>version" lines (dpkg-query/rpm/registry).
func parseTabPackages(output string) []Package {
	var pkgs []Package
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		name, ver, ok := strings.Cut(line, "\t")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			continue
		}
		pkgs = append(pkgs, Package{Name: name, Version: strings.TrimSpace(ver)})
	}
	return pkgs
}

// parseLines turns each non-empty line into a versionless Package (pkgutil).
func parseLines(output string) []Package {
	var pkgs []Package
	for _, line := range strings.Split(output, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			pkgs = append(pkgs, Package{Name: line})
		}
	}
	return pkgs
}

// parseBrewVersions parses `brew list --versions` ("name 1.2 1.3"), keeping the
// first version listed.
func parseBrewVersions(output string) []Package {
	var pkgs []Package
	for _, line := range strings.Split(output, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		p := Package{Name: f[0]}
		if len(f) > 1 {
			p.Version = f[1]
		}
		pkgs = append(pkgs, p)
	}
	return pkgs
}

// parseSystemctl parses `systemctl list-units --type=service --plain` rows
// (UNIT LOAD ACTIVE SUB DESCRIPTION).
func parseSystemctl(output string) []Service {
	var svcs []Service
	for _, line := range strings.Split(output, "\n") {
		f := strings.Fields(line)
		if len(f) < 4 || !strings.HasSuffix(f[0], ".service") {
			continue
		}
		status := "stopped"
		if f[2] == "active" && f[3] == "running" {
			status = "running"
		}
		svcs = append(svcs, Service{Name: strings.TrimSuffix(f[0], ".service"), Status: status})
	}
	return svcs
}

// parseLaunchctl parses `launchctl list` rows (PID STATUS LABEL); a numeric PID
// means the service is running.
func parseLaunchctl(output string) []Service {
	var svcs []Service
	for i, line := range strings.Split(output, "\n") {
		if i == 0 {
			continue // header
		}
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		status := "stopped"
		if _, err := strconv.Atoi(f[0]); err == nil {
			status = "running"
		}
		svcs = append(svcs, Service{Name: f[2], Status: status})
	}
	return svcs
}

// parseVMStat estimates used memory (MB) from macOS `vm_stat` output by summing
// active, wired and compressed pages.
func parseVMStat(output string, pageSize uint64) uint64 {
	get := func(label string) uint64 {
		for _, line := range strings.Split(output, "\n") {
			if strings.HasPrefix(line, label) {
				s := strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(line, label)), ".")
				v, _ := strconv.ParseUint(s, 10, 64)
				return v
			}
		}
		return 0
	}
	pages := get("Pages active:") + get("Pages wired down:") + get("Pages occupied by compressor:")
	return pages * pageSize / (1024 * 1024)
}

// parseWinDisks parses "DeviceID<TAB>SizeBytes<TAB>FreeBytes" lines from the
// Windows agent's Win32_LogicalDisk query.
func parseWinDisks(output string) []Disk {
	var disks []Disk
	for _, line := range strings.Split(output, "\n") {
		f := strings.Split(strings.TrimSpace(line), "\t")
		if len(f) < 3 {
			continue
		}
		total, err1 := strconv.ParseUint(strings.TrimSpace(f[1]), 10, 64)
		free, err2 := strconv.ParseUint(strings.TrimSpace(f[2]), 10, 64)
		if err1 != nil || err2 != nil || total == 0 {
			continue
		}
		disks = append(disks, Disk{Mount: strings.TrimSpace(f[0]), TotalMB: total / (1024 * 1024), UsedMB: (total - free) / (1024 * 1024)})
	}
	return disks
}
