package store

import (
	"fmt"
	"time"
)

// Health is a server's computed status plus plain-English reasons.
type Health struct {
	Status  string   `json:"status"` // ok, warning, critical, stale
	Reasons []string `json:"reasons,omitempty"`
}

// StaleAfter is the default time without a report before a server is considered
// stale. It is also the default Thresholds.StaleAfter; per-host config may override it.
const StaleAfter = 5 * time.Minute

// Evaluate turns a server's latest report into a status anyone can read, using
// the supplied thresholds (see Thresholds / HealthConfig.For). A threshold of 0
// for a given signal disables that signal. The alert engine, not Evaluate, is
// responsible for "sustained"/flap-damping; Evaluate scores the latest sample.
func Evaluate(srv *Server, now time.Time, t Thresholds) Health {
	staleAfter := t.StaleAfter
	if staleAfter <= 0 {
		staleAfter = StaleAfter
	}
	if now.Sub(srv.LastSeen) > staleAfter {
		return Health{
			Status:  "stale",
			Reasons: []string{fmt.Sprintf("no report in %s", now.Sub(srv.LastSeen).Round(time.Minute))},
		}
	}

	status := "ok"
	var reasons []string
	bump := func(to string) {
		if rank(to) > rank(status) {
			status = to
		}
	}

	rep := srv.LastReport
	specs := rep.Specs
	for _, d := range specs.Disks {
		if d.TotalMB == 0 {
			continue
		}
		pct := float64(d.UsedMB) / float64(d.TotalMB) * 100
		switch {
		case t.DiskCritPct > 0 && pct >= t.DiskCritPct:
			bump("critical")
			reasons = append(reasons, fmt.Sprintf("disk %s is %.0f%% full", d.Mount, pct))
		case t.DiskWarnPct > 0 && pct >= t.DiskWarnPct:
			bump("warning")
			reasons = append(reasons, fmt.Sprintf("disk %s is %.0f%% full", d.Mount, pct))
		}
	}
	if specs.MemTotalMB > 0 {
		pct := float64(specs.MemUsedMB) / float64(specs.MemTotalMB) * 100
		switch {
		case t.MemCritPct > 0 && pct >= t.MemCritPct:
			bump("critical")
			reasons = append(reasons, fmt.Sprintf("memory %.0f%% used", pct))
		case t.MemWarnPct > 0 && pct >= t.MemWarnPct:
			bump("warning")
			reasons = append(reasons, fmt.Sprintf("memory %.0f%% used", pct))
		}
	}
	if specs.CPUPercent > 0 {
		switch {
		case t.CPUCritPct > 0 && specs.CPUPercent >= t.CPUCritPct:
			bump("critical")
			reasons = append(reasons, fmt.Sprintf("CPU %.0f%% used", specs.CPUPercent))
		case t.CPUWarnPct > 0 && specs.CPUPercent >= t.CPUWarnPct:
			bump("warning")
			reasons = append(reasons, fmt.Sprintf("CPU %.0f%% used", specs.CPUPercent))
		}
	}
	if len(specs.LoadAvg) > 0 {
		cores := specs.CPUCores
		if cores < 1 {
			cores = 1
		}
		load1 := specs.LoadAvg[0]
		perCore := load1 / float64(cores)
		switch {
		case t.LoadCritPerCore > 0 && perCore >= t.LoadCritPerCore:
			bump("critical")
			reasons = append(reasons, fmt.Sprintf("load %.2f (%.2f per core)", load1, perCore))
		case t.LoadWarnPerCore > 0 && perCore >= t.LoadWarnPerCore:
			bump("warning")
			reasons = append(reasons, fmt.Sprintf("load %.2f (%.2f per core)", load1, perCore))
		}
	}
	for _, name := range t.WatchServices {
		found, running := false, false
		for _, sv := range rep.Services {
			if sv.Name == name {
				found = true
				running = sv.Status == "running"
				break
			}
		}
		switch {
		case !found:
			bump("critical")
			reasons = append(reasons, fmt.Sprintf("service %s is not present", name))
		case !running:
			bump("critical")
			reasons = append(reasons, fmt.Sprintf("service %s is stopped", name))
		}
	}
	return Health{Status: status, Reasons: reasons}
}

// WorseThan reports whether status a is strictly more severe than status b
// (ok < warning < critical < stale).
func WorseThan(a, b string) bool { return rank(a) > rank(b) }

func rank(status string) int {
	switch status {
	case "warning":
		return 1
	case "critical":
		return 2
	case "stale":
		return 3
	default:
		return 0
	}
}
