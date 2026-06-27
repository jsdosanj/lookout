package server

import (
	"context"
	"time"

	"github.com/jsdosanj/lookout/internal/check"
	execpkg "github.com/jsdosanj/lookout/internal/collect/exec"
	"github.com/jsdosanj/lookout/internal/plugin"
)

// SetChecks/SetPlugins install the configured port/HTTP checks and custom-check
// plugins. Call once during wiring, before starting the runners (the runner
// goroutines read these slices, which are not mutated afterwards).
func (s *Server) SetChecks(c []check.Check)    { s.checks = c }
func (s *Server) SetPlugins(p []plugin.Plugin) { s.plugins = p }

// RunChecks runs every configured TCP/HTTP check once and feeds each result into
// the alert engine as a synthetic "server" keyed by the check ID. A failing check
// is a critical observation; a passing one resolves it — reusing the same dedupe,
// flap-damping, and escalation the host-health path already gets.
func (s *Server) RunChecks(now time.Time) {
	if !s.alerts.Enabled() || len(s.checks) == 0 {
		return
	}
	for _, c := range s.checks {
		res := c.Run(context.Background()) // Run applies the check's own timeout
		s.alerts.Observe(c.ID, res.Status, res.Reason, now)
	}
}

// RunPlugins runs every configured Nagios-style plugin once through the safe-exec
// runner (fixed argv, no shell) and feeds each result into the alert engine keyed
// by the plugin name. The allow-list is exactly the configured plugin commands, so
// only those operator-declared binaries can ever run. A Nagios UNKNOWN (exit 3) is
// surfaced as a warning so a check that can't determine state is still visible.
func (s *Server) RunPlugins(now time.Time) {
	if !s.alerts.Enabled() || len(s.plugins) == 0 {
		return
	}
	allow := make([]string, 0, len(s.plugins))
	for _, p := range s.plugins {
		allow = append(allow, p.Command)
	}
	runner := execpkg.NewRunner(allow)
	for _, p := range s.plugins {
		res := plugin.Run(context.Background(), runner, p)
		status := res.Status
		if status == "unknown" {
			status = "warning"
		}
		s.alerts.Observe(p.Name, status, res.Reason, now)
	}
}

// StartCheckRunner runs RunChecks on an interval until stop is closed.
func (s *Server) StartCheckRunner(every time.Duration, stop <-chan struct{}) {
	s.startTicker(s.RunChecks, len(s.checks) > 0, every, stop)
}

// StartPluginRunner runs RunPlugins on an interval until stop is closed.
func (s *Server) StartPluginRunner(every time.Duration, stop <-chan struct{}) {
	s.startTicker(s.RunPlugins, len(s.plugins) > 0, every, stop)
}

// startTicker is the shared periodic-runner used by the check and plugin runners
// (mirrors StartSweeper). It does nothing if alerting is off, the cadence is
// non-positive, or there is nothing to run.
func (s *Server) startTicker(run func(time.Time), have bool, every time.Duration, stop <-chan struct{}) {
	if !s.alerts.Enabled() || every <= 0 || !have {
		return
	}
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case now := <-t.C:
				run(now.UTC())
			}
		}
	}()
}
