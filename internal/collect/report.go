// Package collect gathers a host inventory + health report for the Lookout agent.
//
// Design: every OS-specific gatherer (in sys_<os>.go) only reads files or runs
// fixed commands, then hands the raw text to a pure parser in parse.go. The pure
// parsers have no OS dependency, so they are unit-tested on any platform.
package collect

import (
	"runtime"
	"time"
)

// SchemaVersion identifies the report wire format so the control plane can
// evolve it safely.
const SchemaVersion = "1"

// Report is the full picture an agent sends for one host.
type Report struct {
	SchemaVersion string    `json:"schema_version"`
	CollectedAt   time.Time `json:"collected_at"`
	Host          Host      `json:"host"`
	Specs         Specs     `json:"specs"`
	Packages      []Package `json:"packages"`
	Services      []Service `json:"services"`
}

// Host identifies the machine and its operating system.
type Host struct {
	Hostname      string `json:"hostname"`
	OS            string `json:"os"`       // linux, darwin, windows
	Platform      string `json:"platform"` // ubuntu, debian, rocky, rhel, macos, windows
	Version       string `json:"version"`
	Arch          string `json:"arch"`
	Kernel         string `json:"kernel,omitempty"`
	UptimeSeconds  int64  `json:"uptime_seconds"`
	Virtualization string `json:"virtualization,omitempty"` // physical, kvm, vmware, hyperv, ...
	Encryption     string `json:"encryption,omitempty"`     // on, off, "" (unknown) — FileVault/BitLocker/LUKS
}

// Specs is point-in-time hardware + resource usage.
type Specs struct {
	CPUModel   string    `json:"cpu_model"`
	CPUCores   int       `json:"cpu_cores"`
	CPUPercent float64   `json:"cpu_percent"`
	MemTotalMB uint64    `json:"mem_total_mb"`
	MemUsedMB  uint64    `json:"mem_used_mb"`
	LoadAvg    []float64 `json:"load_avg,omitempty"`
	Disks      []Disk    `json:"disks"`
}

// Disk is one mounted filesystem.
type Disk struct {
	Mount   string `json:"mount"`
	FS      string `json:"fs,omitempty"`
	TotalMB uint64 `json:"total_mb"`
	UsedMB  uint64 `json:"used_mb"`
}

// Package is one installed package or application.
type Package struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// Service is one OS service and whether it is running.
type Service struct {
	Name   string `json:"name"`
	Status string `json:"status"` // running, stopped
}

// Collect gathers a full report for the current host. Host and specs are
// required; packages and services are best-effort and never fail the report
// (a server without systemd, or where listing packages is denied, still reports).
func Collect() (*Report, error) {
	host, err := collectHost()
	if err != nil {
		return nil, err
	}
	host.OS = runtime.GOOS
	host.Arch = runtime.GOARCH

	specs, err := collectSpecs()
	if err != nil {
		return nil, err
	}

	pkgs, _ := collectPackages()
	svcs, _ := collectServices()

	return &Report{
		SchemaVersion: SchemaVersion,
		CollectedAt:   time.Now().UTC(),
		Host:          host,
		Specs:         specs,
		Packages:      pkgs,
		Services:      svcs,
	}, nil
}
