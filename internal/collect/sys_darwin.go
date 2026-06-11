//go:build darwin

package collect

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func sysctl(key string) string {
	v, _ := runCmd("sysctl", "-n", key)
	return v
}

func collectHost() (Host, error) {
	h := Host{Platform: "macos"}
	h.Hostname, _ = os.Hostname()
	if v, err := runCmd("sw_vers", "-productVersion"); err == nil {
		h.Version = "macOS " + v
	}
	h.Kernel = sysctl("kern.osrelease")
	// kern.boottime looks like: "{ sec = 1700000000, usec = 0 } ..."
	if bt := sysctl("kern.boottime"); strings.Contains(bt, "sec =") {
		rest := strings.TrimSpace(bt[strings.Index(bt, "sec =")+len("sec ="):])
		if c := strings.IndexByte(rest, ','); c >= 0 {
			rest = strings.TrimSpace(rest[:c])
		}
		if sec, err := strconv.ParseInt(rest, 10, 64); err == nil {
			h.UptimeSeconds = time.Now().Unix() - sec
		}
	}
	if sysctl("kern.hv_vmm_present") == "1" {
		h.Virtualization = "vm"
	} else {
		h.Virtualization = "physical"
	}
	if out, err := runCmd("fdesetup", "status"); err == nil {
		h.Encryption = parseFileVault(out)
	}
	return h, nil
}

func collectSpecs() (Specs, error) {
	var s Specs
	if s.CPUModel = sysctl("machdep.cpu.brand_string"); s.CPUModel == "" {
		s.CPUModel = sysctl("hw.model")
	}
	s.CPUCores, _ = strconv.Atoi(sysctl("hw.ncpu"))
	if memBytes, err := strconv.ParseUint(sysctl("hw.memsize"), 10, 64); err == nil {
		s.MemTotalMB = memBytes / (1024 * 1024)
		if vm, err := runCmd("vm_stat"); err == nil {
			s.MemUsedMB = parseVMStat(vm, pageSize())
		}
	}
	// vm.loadavg looks like: "{ 1.50 1.20 1.10 }"
	s.LoadAvg = parseLoadAvg(strings.Trim(sysctl("vm.loadavg"), "{} "))
	// CPU utilization: `top -l 2` reports a settled second sample.
	if out, err := runCmd("top", "-l", "2", "-n", "0"); err == nil {
		s.CPUPercent = parseTopCPU(out)
	}
	s.Disks = unixDisks()
	return s, nil
}

func pageSize() uint64 {
	if v, err := strconv.ParseUint(sysctl("hw.pagesize"), 10, 64); err == nil {
		return v
	}
	return 4096
}

func collectPackages() ([]Package, error) {
	if _, err := exec.LookPath("brew"); err == nil {
		if out, err := runCmd("brew", "list", "--versions"); err == nil {
			return parseBrewVersions(out), nil
		}
	}
	if out, err := runCmd("pkgutil", "--pkgs"); err == nil {
		return parseLines(out), nil
	}
	return nil, nil
}

func collectServices() ([]Service, error) {
	out, err := runCmd("launchctl", "list")
	if err != nil {
		return nil, err
	}
	return parseLaunchctl(out), nil
}
