// Command lookout-demo generates the static live-demo dashboard into ./docs
// using realistic sample servers. Run it from the repo root:
//
//	go run ./cmd/lookout-demo
//
// Then enable GitHub Pages (source: /docs) to host it.
package main

import (
	"log"
	"time"

	"github.com/jsdosanj/lookout/internal/collect"
	"github.com/jsdosanj/lookout/internal/server"
	"github.com/jsdosanj/lookout/internal/store"
)

func main() {
	now := time.Now().UTC()
	servers := []*store.Server{
		srv("web-01", now.Add(-1*time.Minute), report("web-01", "ubuntu", "Ubuntu 22.04.4 LTS", "linux", "x86_64",
			"Intel Xeon E5-2680 v4", 8, 16384, 6225, []float64{0.42, 0.38, 0.40},
			[]collect.Disk{{Mount: "/", FS: "ext4", TotalMB: 100000, UsedMB: 42000}, {Mount: "/var", FS: "ext4", TotalMB: 200000, UsedMB: 61000}},
			[]collect.Service{{Name: "nginx", Status: "running"}, {Name: "ssh", Status: "running"}, {Name: "cron", Status: "running"}, {Name: "postgresql", Status: "stopped"}},
			142)),
		srv("db-01", now.Add(-30*time.Second), report("db-01", "rocky", "Rocky Linux 9.3", "linux", "x86_64",
			"AMD EPYC 7543", 32, 131072, 92000, []float64{6.1, 5.8, 5.2},
			[]collect.Disk{{Mount: "/", FS: "xfs", TotalMB: 80000, UsedMB: 40000}, {Mount: "/data", FS: "xfs", TotalMB: 1000000, UsedMB: 942000}},
			[]collect.Service{{Name: "postgresql", Status: "running"}, {Name: "ssh", Status: "running"}, {Name: "node_exporter", Status: "running"}},
			208)),
		srv("app-02", now.Add(-2*time.Minute), report("app-02", "debian", "Debian GNU/Linux 12 (bookworm)", "linux", "aarch64",
			"Ampere Altra", 16, 32768, 30310, []float64{2.2, 2.4, 2.1},
			[]collect.Disk{{Mount: "/", FS: "ext4", TotalMB: 120000, UsedMB: 100000}},
			[]collect.Service{{Name: "lookout-app", Status: "running"}, {Name: "ssh", Status: "running"}, {Name: "redis", Status: "running"}},
			176)),
		srv("win-01", now.Add(-1*time.Minute), report("win-01", "windows", "Microsoft Windows Server 2022", "windows", "x86_64",
			"Intel Xeon Gold 6338", 24, 65536, 28900, nil,
			[]collect.Disk{{Mount: "C:", TotalMB: 256000, UsedMB: 140800}, {Mount: "D:", TotalMB: 512000, UsedMB: 120000}},
			[]collect.Service{{Name: "W3SVC", Status: "running"}, {Name: "MSSQLSERVER", Status: "running"}, {Name: "Spooler", Status: "stopped"}},
			94)),
		srv("cache-01", now.Add(-22*time.Minute), report("cache-01", "centos", "CentOS Stream 9", "linux", "x86_64",
			"Intel Xeon E5-2670", 4, 8192, 3100, []float64{0.1, 0.2, 0.2},
			[]collect.Disk{{Mount: "/", FS: "xfs", TotalMB: 50000, UsedMB: 12000}},
			[]collect.Service{{Name: "redis", Status: "running"}, {Name: "ssh", Status: "running"}},
			88)),
	}

	if err := server.WriteStaticDemo("docs", servers, now); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote static demo for %d servers to ./docs", len(servers))
}

func srv(id string, lastSeen time.Time, rep collect.Report) *store.Server {
	return &store.Server{ID: id, LastSeen: lastSeen, LastReport: rep}
}

func report(host, platform, version, os, arch, cpu string, cores int, memTotal, memUsed uint64,
	load []float64, disks []collect.Disk, services []collect.Service, packages int) collect.Report {
	pkgs := make([]collect.Package, packages) // count is what the UI shows
	return collect.Report{
		SchemaVersion: collect.SchemaVersion,
		CollectedAt:   lastReportTime(),
		Host: collect.Host{
			Hostname: host, OS: os, Platform: platform, Version: version, Arch: arch,
			Kernel: "6.5.0", UptimeSeconds: 1187400,
		},
		Specs: collect.Specs{
			CPUModel: cpu, CPUCores: cores, MemTotalMB: memTotal, MemUsedMB: memUsed,
			LoadAvg: load, Disks: disks,
		},
		Packages: pkgs,
		Services: services,
	}
}

func lastReportTime() time.Time { return time.Now().UTC() }
