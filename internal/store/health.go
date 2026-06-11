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

// StaleAfter is how long without a report before a server is considered stale.
const StaleAfter = 5 * time.Minute

// Evaluate turns a server's latest report into a status anyone can read.
func Evaluate(srv *Server, now time.Time) Health {
	if now.Sub(srv.LastSeen) > StaleAfter {
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

	specs := srv.LastReport.Specs
	for _, d := range specs.Disks {
		if d.TotalMB == 0 {
			continue
		}
		pct := float64(d.UsedMB) / float64(d.TotalMB) * 100
		switch {
		case pct >= 90:
			bump("critical")
			reasons = append(reasons, fmt.Sprintf("disk %s is %.0f%% full", d.Mount, pct))
		case pct >= 80:
			bump("warning")
			reasons = append(reasons, fmt.Sprintf("disk %s is %.0f%% full", d.Mount, pct))
		}
	}
	if specs.MemTotalMB > 0 {
		if pct := float64(specs.MemUsedMB) / float64(specs.MemTotalMB) * 100; pct >= 90 {
			bump("warning")
			reasons = append(reasons, fmt.Sprintf("memory %.0f%% used", pct))
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
