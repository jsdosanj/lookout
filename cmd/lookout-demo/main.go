// Command lookout-demo generates the static live-demo dashboard into ./docs
// using realistic sample servers. Run it from the repo root:
//
//	go run ./cmd/lookout-demo
//
// Then enable GitHub Pages (source: /docs) to host it.
package main

import (
	"log"
	"math"
	"time"

	"github.com/jsdosanj/lookout/internal/collect"
	"github.com/jsdosanj/lookout/internal/server"
	"github.com/jsdosanj/lookout/internal/store"
)

func main() {
	now := time.Now().UTC()
	servers := []*store.Server{
		srv("web-01", now.Add(-1*time.Minute), report("web-01", "ubuntu", "Ubuntu 22.04.4 LTS", "linux", "x86_64", "physical",
			"Intel Xeon E5-2680 v4", 8, 38, 16384, 6225, []float64{0.42, 0.38, 0.40},
			[]collect.Disk{{Mount: "/", FS: "ext4", TotalMB: 100000, UsedMB: 42000}, {Mount: "/var", FS: "ext4", TotalMB: 200000, UsedMB: 61000}},
			[]collect.Service{{Name: "nginx", Status: "running"}, {Name: "ssh", Status: "running"}, {Name: "cron", Status: "running"}, {Name: "postgresql", Status: "stopped"}}, 142), now),
		srv("db-01", now.Add(-30*time.Second), report("db-01", "rocky", "Rocky Linux 9.3", "linux", "x86_64", "kvm",
			"AMD EPYC 7543", 32, 72, 131072, 92000, []float64{6.1, 5.8, 5.2},
			[]collect.Disk{{Mount: "/", FS: "xfs", TotalMB: 80000, UsedMB: 40000}, {Mount: "/data", FS: "xfs", TotalMB: 1000000, UsedMB: 942000}},
			[]collect.Service{{Name: "postgresql", Status: "running"}, {Name: "ssh", Status: "running"}, {Name: "node_exporter", Status: "running"}}, 208), now),
		srv("app-02", now.Add(-2*time.Minute), report("app-02", "debian", "Debian GNU/Linux 12 (bookworm)", "linux", "aarch64", "physical",
			"Ampere Altra", 16, 64, 32768, 30310, []float64{2.2, 2.4, 2.1},
			[]collect.Disk{{Mount: "/", FS: "ext4", TotalMB: 120000, UsedMB: 100000}},
			[]collect.Service{{Name: "lookout-app", Status: "running"}, {Name: "ssh", Status: "running"}, {Name: "redis", Status: "running"}}, 176), now),
		srv("win-01", now.Add(-1*time.Minute), report("win-01", "windows", "Microsoft Windows Server 2022", "windows", "x86_64", "hyperv",
			"Intel Xeon Gold 6338", 24, 41, 65536, 28900, nil,
			[]collect.Disk{{Mount: "C:", TotalMB: 256000, UsedMB: 140800}, {Mount: "D:", TotalMB: 512000, UsedMB: 120000}},
			[]collect.Service{{Name: "W3SVC", Status: "running"}, {Name: "MSSQLSERVER", Status: "running"}, {Name: "Spooler", Status: "stopped"}}, 94), now),
		srv("cache-01", now.Add(-22*time.Minute), report("cache-01", "centos", "CentOS Stream 9", "linux", "x86_64", "vmware",
			"Intel Xeon E5-2670", 4, 12, 8192, 3100, []float64{0.1, 0.2, 0.2},
			[]collect.Disk{{Mount: "/", FS: "xfs", TotalMB: 50000, UsedMB: 12000}},
			[]collect.Service{{Name: "redis", Status: "running"}, {Name: "ssh", Status: "running"}}, 88), now),
	}

	if err := server.WriteStaticDemo("docs", servers, now); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote static demo for %d servers to ./docs", len(servers))
}

func srv(id string, lastSeen time.Time, rep collect.Report, now time.Time) *store.Server {
	var memPct, diskPct float64
	if rep.Specs.MemTotalMB > 0 {
		memPct = float64(rep.Specs.MemUsedMB) / float64(rep.Specs.MemTotalMB) * 100
	}
	for _, d := range rep.Specs.Disks {
		if d.TotalMB > 0 {
			if p := float64(d.UsedMB) / float64(d.TotalMB) * 100; p > diskPct {
				diskPct = p
			}
		}
	}
	return &store.Server{ID: id, LastSeen: lastSeen, LastReport: rep, History: genHistory(now, rep.Specs.CPUPercent, memPct, diskPct)}
}

// genHistory fabricates ~40 minutes of plausible samples wobbling around each
// server's current values (deterministic, no randomness).
func genHistory(now time.Time, cpu, mem, disk float64) []store.Sample {
	var h []store.Sample
	for i := 39; i >= 0; i-- {
		t := now.Add(-time.Duration(i) * time.Minute)
		h = append(h, store.Sample{
			At:   t,
			CPU:  clamp(cpu + math.Sin(float64(i)/3)*10 + math.Cos(float64(i)/2)*5),
			Mem:  clamp(mem + math.Sin(float64(i)/6)*4),
			Disk: clamp(disk - float64(i)*0.05),
		})
	}
	return h
}

func clamp(f float64) float64 {
	switch {
	case f < 0:
		return 0
	case f > 100:
		return 100
	default:
		return math.Round(f*10) / 10
	}
}

func report(host, platform, version, os, arch, virt, cpu string, cores int, cpuPct float64, memTotal, memUsed uint64,
	load []float64, disks []collect.Disk, services []collect.Service, packages int) collect.Report {
	return collect.Report{
		SchemaVersion: collect.SchemaVersion,
		CollectedAt:   time.Now().UTC(),
		Host: collect.Host{
			Hostname: host, OS: os, Platform: platform, Version: version, Arch: arch,
			Kernel: "6.5.0", UptimeSeconds: 1187400, Virtualization: virt,
		},
		Specs: collect.Specs{
			CPUModel: cpu, CPUCores: cores, CPUPercent: cpuPct, MemTotalMB: memTotal, MemUsedMB: memUsed,
			LoadAvg: load, Disks: disks,
		},
		Packages: make([]collect.Package, packages),
		Services: services,
	}
}
