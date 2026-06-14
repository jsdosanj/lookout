// Package server is the Lookout control plane: it ingests agent reports and
// serves the dashboard.
package server

import (
	"crypto/subtle"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/jsdosanj/lookout/internal/auth"
	"github.com/jsdosanj/lookout/internal/collect"
	"github.com/jsdosanj/lookout/internal/notify"
	"github.com/jsdosanj/lookout/internal/store"
)

// Server wires the HTTP handlers to the store.
type Server struct {
	store        *store.Store
	agents       *store.AgentStore // per-agent credentials + TOFU hostname pins
	token        string            // shared agent enrollment token
	requireAgent bool              // when true, the legacy shared token is rejected on /report
	auth         *auth.Auth        // user auth; nil disables login (dev only)
	notifier     *notify.Notifier  // alert delivery (webhook/Slack/Teams); may be nil
}

// New creates a control-plane server. token authenticates agents; a authenticates
// dashboard users (pass nil for no-login dev mode); n delivers alerts (may be nil).
// ag holds per-agent credentials; requireAgent locks down the legacy shared token.
func New(st *store.Store, ag *store.AgentStore, token string, requireAgent bool, a *auth.Auth, n *notify.Notifier) *Server {
	return &Server{store: st, agents: ag, token: token, requireAgent: requireAgent, auth: a, notifier: n}
}

// Routes returns the HTTP handler for the control plane.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	// Public + agent endpoints (agents use the enrollment token, not a login).
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("POST /api/v1/agents/enroll", s.handleEnroll)
	mux.HandleFunc("POST /api/v1/agents/report", s.handleReport)

	if s.auth == nil {
		// No-login dev mode.
		mux.HandleFunc("GET /api/v1/servers", s.handleListJSON)
		mux.HandleFunc("GET /server/{id}", s.handleDetail)
		mux.HandleFunc("GET /", s.handleDashboard)
		return securityHeaders(mux)
	}

	// Login/account/admin/OAuth routes.
	s.auth.Mount(mux)
	// Dashboard requires a logged-in user with the view permission.
	view := func(h http.HandlerFunc) http.Handler { return s.auth.RequirePermission(auth.PermViewDashboard, h) }
	mux.Handle("GET /api/v1/servers", view(s.handleListJSON))
	mux.Handle("GET /server/{id}", view(s.handleDetail))
	mux.Handle("GET /guides", view(s.handleGuides))
	mux.Handle("GET /integrations", view(s.handleIntegrations))
	mux.Handle("GET /integrations/{id}", view(s.handleIntegrationDetail))
	mux.Handle("GET /notifications", view(s.handleNotifications))
	mux.Handle("GET /settings", view(s.handleSettings))
	mux.Handle("GET /{$}", view(s.handleDashboard))
	return securityHeaders(mux)
}

// securityHeaders wraps h with baseline browser hardening headers. The CSP pins
// script sources to ourselves plus the Chart.js CDN host and forbids framing.
func securityHeaders(h http.Handler) http.Handler {
	const csp = "default-src 'self'; " +
		"script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; " +
		"style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data:; " +
		"frame-ancestors 'none'; base-uri 'self'"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hd := w.Header()
		hd.Set("X-Frame-Options", "DENY")
		hd.Set("X-Content-Type-Options", "nosniff")
		hd.Set("Referrer-Policy", "no-referrer")
		hd.Set("Content-Security-Policy", csp)
		if r.TLS != nil {
			hd.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		h.ServeHTTP(w, r)
	})
}

// handleEnroll issues a per-agent token. The caller must present the shared
// enrollment token (LOOKOUT_TOKEN); the response binds a server-assigned agent
// identity to a freshly generated per-agent token. This is the migration path:
// agents enroll once, then report with their own token.
func (s *Server) handleEnroll(w http.ResponseWriter, r *http.Request) {
	if !s.sharedTokenOK(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req struct {
		Hostname string `json:"hostname"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(&req); err != nil && err.Error() != "EOF" {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	id, token, err := s.agents.Enroll(req.Hostname)
	if err != nil {
		http.Error(w, "enroll error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"agent_id": id, "agent_token": token})
}

// handleReport ingests one agent report. The reporting identity is resolved from
// the credential: a per-agent token binds to its assigned identity; the legacy
// shared token reports as the single "shared" identity. The hostname is pinned
// to the first identity that reports it (TOFU) and a conflicting identity is
// rejected, so no token holder can overwrite another host's record.
func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.reportIdentity(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var rep collect.Report
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20)) // 8 MiB cap (DoS guard)
	if err := dec.Decode(&rep); err != nil {
		http.Error(w, "bad report: "+err.Error(), http.StatusBadRequest)
		return
	}
	if rep.Host.Hostname == "" {
		http.Error(w, "missing host.hostname", http.StatusBadRequest)
		return
	}
	// TOFU: pin the hostname to this identity (or reject if already pinned to a
	// different one). This blocks cross-host overwrite even under the shared token.
	if err := s.agents.BindHostname(rep.Host.Hostname, identity); err != nil {
		log.Printf("report rejected: %v (identity=%s)", err, identity)
		http.Error(w, "hostname is claimed by another agent", http.StatusConflict)
		return
	}
	// Capture the previous health so we can alert if this report makes it worse.
	now := time.Now().UTC()
	var prev string
	if old, ok := s.store.Get(rep.Host.Hostname); ok {
		prev = store.Evaluate(old, now).Status
	}
	if err := s.store.Save(&rep); err != nil {
		http.Error(w, "store error", http.StatusInternalServerError)
		return
	}
	s.maybeAlert(rep.Host.Hostname, prev, now)
	w.WriteHeader(http.StatusNoContent)
}

// maybeAlert fires a notification when a server's health worsens into a
// warning/critical state on this report.
func (s *Server) maybeAlert(id, prev string, now time.Time) {
	if !s.notifier.Enabled() {
		return
	}
	srv, ok := s.store.Get(id)
	if !ok {
		return
	}
	h := store.Evaluate(srv, now)
	if store.WorseThan(h.Status, prev) && (h.Status == "warning" || h.Status == "critical") {
		reason := ""
		if len(h.Reasons) > 0 {
			reason = h.Reasons[0]
		}
		s.notifier.Notify(notify.Event{Server: id, OldStatus: prev, NewStatus: h.Status, Reason: reason})
	}
}

// bearer extracts the token from an "Authorization: Bearer <token>" header.
func bearer(r *http.Request) string {
	const p = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) <= len(p) || h[:len(p)] != p {
		return ""
	}
	return h[len(p):]
}

// sharedTokenOK checks the legacy shared token in constant time. When no token
// is configured (dev mode) it allows the request.
func (s *Server) sharedTokenOK(r *http.Request) bool {
	if s.token == "" {
		return true
	}
	got := []byte(r.Header.Get("Authorization"))
	want := []byte("Bearer " + s.token)
	return subtle.ConstantTimeCompare(got, want) == 1
}

// reportIdentity authenticates a /report request and returns the reporting
// agent identity. A valid per-agent token resolves to its bound identity; the
// legacy shared token resolves to store.SharedIdentity (unless requireAgent
// disables it). Dev mode (no shared token, no per-agent match) reports as shared.
func (s *Server) reportIdentity(r *http.Request) (string, bool) {
	tok := bearer(r)
	if a, ok := s.agents.AgentByToken(tok); ok {
		return a.ID, true
	}
	if s.requireAgent {
		// Fleet locked down to per-agent tokens: the shared token is not accepted.
		return "", false
	}
	if s.sharedTokenOK(r) {
		return store.SharedIdentity, true
	}
	return "", false
}

func (s *Server) handleListJSON(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	type item struct {
		*store.Server
		Health store.Health `json:"health"`
	}
	servers := s.store.List()
	out := make([]item, 0, len(servers))
	for _, srv := range servers {
		out = append(out, item{Server: srv, Health: store.Evaluate(srv, now)})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// csrfField renders the hidden CSRF input for the dashboard's POST forms (the
// sign-out form). tok is the session synchronizer token from auth.CSRFToken.
func csrfField(tok string) template.HTML {
	return template.HTML(`<input type="hidden" name="` + auth.CSRFField + `" value="` + template.HTMLEscapeString(tok) + `">`)
}

func percent(used, total uint64) int {
	if total == 0 {
		return 0
	}
	return int(float64(used) / float64(total) * 100)
}
