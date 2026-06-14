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

	"github.com/jsdosanj/lookout/internal/alert"
	"github.com/jsdosanj/lookout/internal/auth"
	"github.com/jsdosanj/lookout/internal/collect"
	"github.com/jsdosanj/lookout/internal/store"
)

// Server wires the HTTP handlers to the store.
type Server struct {
	store        *store.Store
	agents       *store.AgentStore // per-agent credentials + TOFU hostname pins
	token        string            // shared agent enrollment token
	requireAgent bool              // when true, the legacy shared token is rejected on /report
	auth         *auth.Auth        // user auth; nil disables login (dev only)
	alerts       *alert.Engine     // alert rule engine + delivery; may be nil
	activity     *alert.Recorder   // recent alert deliveries for the UI; may be nil
	rules        *alert.RuleStore  // persisted, dashboard-editable rules; may be nil
}

// New creates a control-plane server. token authenticates agents; a authenticates
// dashboard users (pass nil for no-login dev mode); eng evaluates alert rules and
// delivers notifications (may be nil); rec exposes recent deliveries to the UI
// (may be nil); rs persists dashboard-editable rules (may be nil). ag holds
// per-agent credentials; requireAgent locks down the legacy shared token.
func New(st *store.Store, ag *store.AgentStore, token string, requireAgent bool, a *auth.Auth, eng *alert.Engine, rec *alert.Recorder, rs *alert.RuleStore) *Server {
	return &Server{store: st, agents: ag, token: token, requireAgent: requireAgent, auth: a, alerts: eng, activity: rec, rules: rs}
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
	// Alert-rule and acknowledgement actions: editing alerting is privileged and
	// CSRF-protected (state-changing POSTs from the dashboard).
	manage := func(h http.HandlerFunc) http.Handler { return s.auth.ProtectPost(auth.PermManageAlerts, h) }
	mux.Handle("POST /notifications/rules/save", manage(s.handleRuleSave))
	mux.Handle("POST /notifications/rules/delete", manage(s.handleRuleDelete))
	mux.Handle("POST /notifications/ack", manage(s.handleAck))
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
	now := time.Now().UTC()
	if err := s.store.Save(&rep); err != nil {
		http.Error(w, "store error", http.StatusInternalServerError)
		return
	}
	s.maybeAlert(rep.Host.Hostname, now)
	w.WriteHeader(http.StatusNoContent)
}

// maybeAlert feeds this report's health into the alert engine, which owns the
// rule evaluation, dedupe, flap-damping, and escalation decisions (it fires only
// when a rule says to). prev is unused now that the engine tracks incident state.
func (s *Server) maybeAlert(id string, now time.Time) {
	if !s.alerts.Enabled() {
		return
	}
	srv, ok := s.store.Get(id)
	if !ok {
		return
	}
	h := store.Evaluate(srv, now)
	reason := ""
	if len(h.Reasons) > 0 {
		reason = h.Reasons[0]
	}
	s.alerts.Observe(id, h.Status, reason, now)
}

// Sweep re-evaluates every known server's health at time now and feeds it to the
// alert engine. This is what makes a SILENT host alert: alerts otherwise fire
// only on report ingest, so a host that stops reporting (and thus turns "stale"
// after StaleAfter) would never trigger anything without this periodic pass. The
// engine's dedupe means a sweep that finds nothing newly wrong is a no-op.
func (s *Server) Sweep(now time.Time) {
	if !s.alerts.Enabled() {
		return
	}
	for _, srv := range s.store.List() {
		h := store.Evaluate(srv, now)
		reason := ""
		if len(h.Reasons) > 0 {
			reason = h.Reasons[0]
		}
		s.alerts.Observe(srv.ID, h.Status, reason, now)
	}
}

// StartSweeper runs Sweep on an interval until stop is closed. It is the
// background half of stale detection (the report path covers live hosts; this
// covers hosts that have gone silent).
func (s *Server) StartSweeper(every time.Duration, stop <-chan struct{}) {
	if !s.alerts.Enabled() || every <= 0 {
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
				s.Sweep(now.UTC())
			}
		}
	}()
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
