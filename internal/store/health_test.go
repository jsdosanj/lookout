package store

import (
	"testing"
	"time"

	"github.com/jsdosanj/lookout/internal/collect"
)

// srvWith builds a fresh (non-stale) server with the given specs and services.
func srvWith(specs collect.Specs, svcs []collect.Service) *Server {
	return &Server{
		ID:       "h1",
		LastSeen: time.Now().UTC(),
		LastReport: collect.Report{
			Host:     collect.Host{Hostname: "h1"},
			Specs:    specs,
			Services: svcs,
		},
	}
}

func TestEvaluateCPU(t *testing.T) {
	now := time.Now().UTC()
	def := DefaultThresholds()

	if h := Evaluate(srvWith(collect.Specs{CPUPercent: 50}, nil), now, def); h.Status != "ok" {
		t.Errorf("CPU 50%%: want ok, got %s", h.Status)
	}
	if h := Evaluate(srvWith(collect.Specs{CPUPercent: 90}, nil), now, def); h.Status != "warning" {
		t.Errorf("CPU 90%%: want warning, got %s (%v)", h.Status, h.Reasons)
	}
	if h := Evaluate(srvWith(collect.Specs{CPUPercent: 97}, nil), now, def); h.Status != "critical" {
		t.Errorf("CPU 97%%: want critical, got %s", h.Status)
	}
}

func TestEvaluateLoad(t *testing.T) {
	now := time.Now().UTC()
	def := DefaultThresholds()

	// load 1.5 on 2 cores = 0.75/core -> ok (< 1.0 warn).
	if h := Evaluate(srvWith(collect.Specs{CPUCores: 2, LoadAvg: []float64{1.5}}, nil), now, def); h.Status != "ok" {
		t.Errorf("load 0.75/core: want ok, got %s", h.Status)
	}
	// load 3.0 on 2 cores = 1.5/core -> warning.
	if h := Evaluate(srvWith(collect.Specs{CPUCores: 2, LoadAvg: []float64{3.0}}, nil), now, def); h.Status != "warning" {
		t.Errorf("load 1.5/core: want warning, got %s", h.Status)
	}
	// load 5.0 on 2 cores = 2.5/core -> critical.
	if h := Evaluate(srvWith(collect.Specs{CPUCores: 2, LoadAvg: []float64{5.0}}, nil), now, def); h.Status != "critical" {
		t.Errorf("load 2.5/core: want critical, got %s", h.Status)
	}
	// zero cores must not divide-by-zero (treated as 1 core): load 0.5 -> ok.
	if h := Evaluate(srvWith(collect.Specs{CPUCores: 0, LoadAvg: []float64{0.5}}, nil), now, def); h.Status != "ok" {
		t.Errorf("load with 0 cores: want ok, got %s", h.Status)
	}
}

func TestEvaluateWatchedService(t *testing.T) {
	now := time.Now().UTC()
	t.Run("stopped", func(t *testing.T) {
		th := Thresholds{WatchServices: []string{"nginx"}}
		svcs := []collect.Service{{Name: "nginx", Status: "stopped"}}
		h := Evaluate(srvWith(collect.Specs{}, svcs), now, th)
		if h.Status != "critical" || len(h.Reasons) == 0 || h.Reasons[0] != "service nginx is stopped" {
			t.Errorf("stopped service: got %s %v", h.Status, h.Reasons)
		}
	})
	t.Run("absent", func(t *testing.T) {
		th := Thresholds{WatchServices: []string{"nginx"}}
		h := Evaluate(srvWith(collect.Specs{}, nil), now, th)
		if h.Status != "critical" || h.Reasons[0] != "service nginx is not present" {
			t.Errorf("absent service: got %s %v", h.Status, h.Reasons)
		}
	})
	t.Run("running", func(t *testing.T) {
		th := Thresholds{WatchServices: []string{"nginx"}}
		svcs := []collect.Service{{Name: "nginx", Status: "running"}}
		if h := Evaluate(srvWith(collect.Specs{}, svcs), now, th); h.Status != "ok" {
			t.Errorf("running watched service: want ok, got %s", h.Status)
		}
	})
}

// TestEvaluateDefaultsUnchanged pins the historical disk/mem behavior so the
// move to configurable thresholds did not regress the out-of-the-box product.
func TestEvaluateDefaultsUnchanged(t *testing.T) {
	now := time.Now().UTC()
	def := DefaultThresholds()
	disk := func(used uint64) collect.Specs {
		return collect.Specs{Disks: []collect.Disk{{Mount: "/", TotalMB: 100, UsedMB: used}}}
	}
	if h := Evaluate(srvWith(disk(85), nil), now, def); h.Status != "warning" {
		t.Errorf("disk 85%%: want warning, got %s", h.Status)
	}
	if h := Evaluate(srvWith(disk(95), nil), now, def); h.Status != "critical" {
		t.Errorf("disk 95%%: want critical, got %s", h.Status)
	}
	// memory still only warns at 90 (no default critical).
	mem := collect.Specs{MemTotalMB: 100, MemUsedMB: 99}
	if h := Evaluate(srvWith(mem, nil), now, def); h.Status != "warning" {
		t.Errorf("mem 99%%: want warning (no default crit), got %s", h.Status)
	}
}

func TestEvaluateStaleOverride(t *testing.T) {
	now := time.Now().UTC()
	srv := srvWith(collect.Specs{}, nil)
	srv.LastSeen = now.Add(-2 * time.Minute)
	// Default 5m window: still fresh.
	if h := Evaluate(srv, now, DefaultThresholds()); h.Status != "ok" {
		t.Errorf("2m old with 5m window: want ok, got %s", h.Status)
	}
	// Tightened 1m window: now stale.
	if h := Evaluate(srv, now, Thresholds{StaleAfter: time.Minute}); h.Status != "stale" {
		t.Errorf("2m old with 1m window: want stale, got %s", h.Status)
	}
}

// TestHealthConfigResolution checks the defaults -> group -> host override layering.
func TestHealthConfigResolution(t *testing.T) {
	cfg := &HealthConfig{
		Defaults:  Thresholds{DiskCritPct: 90},
		Groups:    map[string]Thresholds{"db": {DiskCritPct: 95}},
		Hosts:     map[string]Thresholds{"special": {DiskCritPct: 70}},
		HostGroup: map[string]string{"db-01": "db", "special": "db"},
	}
	if got := cfg.For("unknown-host").DiskCritPct; got != 90 {
		t.Errorf("unknown host should get default 90, got %v", got)
	}
	if got := cfg.For("db-01").DiskCritPct; got != 95 {
		t.Errorf("db-01 should inherit group 95, got %v", got)
	}
	// host override wins over its group.
	if got := cfg.For("special").DiskCritPct; got != 70 {
		t.Errorf("special host override should be 70, got %v", got)
	}
	// nil config yields package defaults.
	if got := (*HealthConfig)(nil).For("x").DiskCritPct; got != DefaultThresholds().DiskCritPct {
		t.Errorf("nil config should yield package default, got %v", got)
	}
	// a partial host override keeps inherited fields (CPU default preserved).
	if got := cfg.For("special").CPUWarnPct; got != DefaultThresholds().CPUWarnPct {
		t.Errorf("partial override must keep inherited CPU default, got %v", got)
	}
}
