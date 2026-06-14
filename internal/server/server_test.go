package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jsdosanj/lookout/internal/store"
)

// newTestServer builds a server with isolated stores. auth is nil (no login),
// which is fine: these tests exercise the agent report/enroll path only.
func newTestServer(t *testing.T, token string, requireAgent bool) *Server {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "data.json"))
	if err != nil {
		t.Fatal(err)
	}
	ag, err := store.OpenAgents(filepath.Join(t.TempDir(), "agents.json"))
	if err != nil {
		t.Fatal(err)
	}
	return New(st, ag, token, requireAgent, nil, nil)
}

func reportBody(hostname string) string {
	return `{"schema_version":"1","host":{"hostname":"` + hostname + `","os":"linux"}}`
}

func postReport(s *Server, token, hostname string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/report", strings.NewReader(reportBody(hostname)))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleReport(w, req)
	return w
}

func TestReportRejectsBadToken(t *testing.T) {
	s := newTestServer(t, "shared-secret", false)
	if w := postReport(s, "wrong", "h1"); w.Code != http.StatusUnauthorized {
		t.Errorf("bad token: want 401, got %d", w.Code)
	}
	if w := postReport(s, "", "h1"); w.Code != http.StatusUnauthorized {
		t.Errorf("missing token: want 401, got %d", w.Code)
	}
	if w := postReport(s, "shared-secret", "h1"); w.Code != http.StatusNoContent {
		t.Errorf("legacy shared token: want 204, got %d", w.Code)
	}
}

func TestReportPerAgentToken(t *testing.T) {
	s := newTestServer(t, "shared-secret", false)
	_, tok, err := s.agents.Enroll("agent-host")
	if err != nil {
		t.Fatal(err)
	}
	if w := postReport(s, tok, "agent-host"); w.Code != http.StatusNoContent {
		t.Errorf("per-agent token report: want 204, got %d", w.Code)
	}
}

func TestReportRequireAgentTokenLocksOutShared(t *testing.T) {
	s := newTestServer(t, "shared-secret", true)
	// Shared token must be rejected when requireAgent is on.
	if w := postReport(s, "shared-secret", "h1"); w.Code != http.StatusUnauthorized {
		t.Errorf("shared token in lockdown: want 401, got %d", w.Code)
	}
	// A per-agent token still works.
	_, tok, _ := s.agents.Enroll("h1")
	if w := postReport(s, tok, "h1"); w.Code != http.StatusNoContent {
		t.Errorf("per-agent in lockdown: want 204, got %d", w.Code)
	}
}

func TestReportCrossHostOverwriteRejected(t *testing.T) {
	s := newTestServer(t, "shared-secret", false)
	// agentA pins "victim".
	_, tokA, _ := s.agents.Enroll("victim")
	if w := postReport(s, tokA, "victim"); w.Code != http.StatusNoContent {
		t.Fatalf("agentA first report: want 204, got %d", w.Code)
	}
	// agentB tries to overwrite "victim" -> must be rejected (409).
	_, tokB, _ := s.agents.Enroll("other")
	if w := postReport(s, tokB, "victim"); w.Code != http.StatusConflict {
		t.Errorf("cross-host overwrite: want 409, got %d", w.Code)
	}
}

func TestEnrollIssuesToken(t *testing.T) {
	s := newTestServer(t, "shared-secret", false)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/enroll", strings.NewReader(`{"hostname":"new-host"}`))
	req.Header.Set("Authorization", "Bearer shared-secret")
	w := httptest.NewRecorder()
	s.handleEnroll(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("enroll: want 200, got %d (%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "agent_token") {
		t.Errorf("enroll response missing agent_token: %s", w.Body.String())
	}
	// Enroll requires the shared token.
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/agents/enroll", strings.NewReader(`{}`))
	req2.Header.Set("Authorization", "Bearer wrong")
	w2 := httptest.NewRecorder()
	s.handleEnroll(w2, req2)
	if w2.Code != http.StatusUnauthorized {
		t.Errorf("enroll with bad token: want 401, got %d", w2.Code)
	}
}
