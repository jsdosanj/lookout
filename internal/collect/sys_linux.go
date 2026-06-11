//go:build linux

package collect

import (
	"os"
	"os/exec"
	"time"
)

func collectHost() (Host, error) {
	h := Host{Platform: "linux"}
	h.Hostname, _ = os.Hostname()
	if b, err := os.ReadFile("/etc/os-release"); err == nil {
		if id, ver := parseOSRelease(string(b)); id != "" {
			h.Platform, h.Version = id, ver
		}
	}
	if b, err := os.ReadFile("/proc/uptime"); err == nil {
		h.UptimeSeconds = parseUptime(string(b))
	}
	if k, err := runCmd("uname", "-r"); err == nil {
		h.Kernel = k
	}
	if v, err := runCmd("systemd-detect-virt"); err == nil {
		h.Virtualization = normVirt(v)
	}
	if out, err := runCmd("lsblk", "-rno", "TYPE"); err == nil {
		h.Encryption = parseLsblkCrypt(out)
	}
	return h, nil
}

func collectSpecs() (Specs, error) {
	var s Specs
	if b, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		s.CPUModel, s.CPUCores = parseCPUInfo(string(b))
	}
	if b, err := os.ReadFile("/proc/meminfo"); err == nil {
		s.MemTotalMB, s.MemUsedMB = parseMeminfo(string(b))
	}
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		s.LoadAvg = parseLoadAvg(string(b))
	}
	// CPU utilization: sample /proc/stat twice over a short window.
	if b1, err := os.ReadFile("/proc/stat"); err == nil {
		i1, t1 := parseProcStatCPU(string(b1))
		time.Sleep(300 * time.Millisecond)
		if b2, err := os.ReadFile("/proc/stat"); err == nil {
			i2, t2 := parseProcStatCPU(string(b2))
			s.CPUPercent = cpuPercentDelta(i1, t1, i2, t2)
		}
	}
	s.Disks = unixDisks()
	return s, nil
}

func collectPackages() ([]Package, error) {
	if _, err := exec.LookPath("dpkg-query"); err == nil {
		if out, err := runCmd("dpkg-query", "-W", "-f=${Package}\t${Version}\n"); err == nil {
			return parseTabPackages(out), nil
		}
	}
	if _, err := exec.LookPath("rpm"); err == nil {
		if out, err := runCmd("rpm", "-qa", "--qf", "%{NAME}\t%{VERSION}-%{RELEASE}\n"); err == nil {
			return parseTabPackages(out), nil
		}
	}
	return nil, nil
}

func collectServices() ([]Service, error) {
	out, err := runCmd("systemctl", "list-units", "--type=service", "--all", "--no-legend", "--no-pager", "--plain")
	if err != nil {
		return nil, err
	}
	return parseSystemctl(out), nil
}
