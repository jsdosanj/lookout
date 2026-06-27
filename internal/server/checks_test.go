package server

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jsdosanj/lookout/internal/alert"
	"github.com/jsdosanj/lookout/internal/check"
	"github.com/jsdosanj/lookout/internal/store"
)

func withEngine(t *testing.T, s *Server) *captureChannel {
	t.Helper()
	cap := &captureChannel{}
	rule := alert.Rule{ID: "r", Name: "r", Server: "*", MinSeverity: alert.SevWarning,
		FlapWindow: 1, Channels: []string{"cap"}}
	s.alerts = alert.NewEngine([]alert.Rule{rule}, []alert.Channel{cap}, nil)
	return cap
}

// TestRunChecksFiresAlert wires a failing TCP check into the alert pipeline.
func TestRunChecksFiresAlert(t *testing.T) {
	s := newTestServer(t, "shared-secret", false)
	cap := withEngine(t, s)

	// A guaranteed-closed port: listen, grab the address, then close it.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	s.SetChecks([]check.Check{{ID: "web-check", Type: "tcp", Target: addr, Timeout: time.Second}})
	s.RunChecks(time.Now().UTC())

	if len(cap.sent) != 1 || cap.sent[0].Server != "web-check" || cap.sent[0].Severity != "critical" {
		t.Fatalf("failing check should fire one critical for web-check, got %+v", cap.sent)
	}
}

// TestRunChecksOKDoesNotAlert confirms a passing check is silent.
func TestRunChecksOKDoesNotAlert(t *testing.T) {
	s := newTestServer(t, "shared-secret", false)
	cap := withEngine(t, s)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	s.SetChecks([]check.Check{{ID: "up", Type: "tcp", Target: ln.Addr().String(), Timeout: time.Second}})
	s.RunChecks(time.Now().UTC())
	if len(cap.sent) != 0 {
		t.Fatalf("passing check should not alert, got %+v", cap.sent)
	}
}

// TestPerHostThresholdReScores proves a per-host override changes the outcome:
// a 60%-full disk is normally OK, but a host override with a 50% critical
// threshold makes the same report fire critical.
func TestPerHostThresholdReScores(t *testing.T) {
	s := newTestServer(t, "shared-secret", false)
	cap := withEngine(t, s)

	cfg := store.DefaultHealthConfig()
	cfg.Hosts = map[string]store.Thresholds{"h1": {DiskCritPct: 50}}
	if err := s.store.SetHealthConfig(cfg); err != nil {
		t.Fatal(err)
	}

	body := `{"schema_version":"1","host":{"hostname":"h1","os":"linux"},` +
		`"specs":{"disks":[{"mount":"/","total_mb":100,"used_mb":60}]}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/report", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer shared-secret")
	req.Header.Set("Content-Type", "application/json")
	s.handleReport(httptest.NewRecorder(), req)

	if len(cap.sent) != 1 || cap.sent[0].Severity != "critical" || cap.sent[0].Server != "h1" {
		t.Fatalf("per-host threshold should make a 60%% disk critical, got %+v", cap.sent)
	}
}
