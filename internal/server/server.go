// Package server is the Lookout control plane: it ingests agent reports and
// serves the dashboard.
package server

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jsdosanj/lookout/internal/auth"
	"github.com/jsdosanj/lookout/internal/collect"
	"github.com/jsdosanj/lookout/internal/store"
)

// Server wires the HTTP handlers to the store.
type Server struct {
	store *store.Store
	token string       // shared agent enrollment token
	auth  *auth.Auth   // user auth; nil disables login (dev only)
}

// New creates a control-plane server. token authenticates agents; a authenticates
// dashboard users (pass nil only for a no-login dev mode).
func New(st *store.Store, token string, a *auth.Auth) *Server {
	return &Server{store: st, token: token, auth: a}
}

// Routes returns the HTTP handler for the control plane.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	// Public + agent endpoints (agents use the enrollment token, not a login).
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("POST /api/v1/agents/report", s.handleReport)

	if s.auth == nil {
		// No-login dev mode.
		mux.HandleFunc("GET /api/v1/servers", s.handleListJSON)
		mux.HandleFunc("GET /server/{id}", s.handleDetail)
		mux.HandleFunc("GET /", s.handleDashboard)
		return mux
	}

	// Login/account/admin/OAuth routes.
	s.auth.Mount(mux)
	// Dashboard requires a logged-in user with the view permission.
	view := func(h http.HandlerFunc) http.Handler { return s.auth.RequirePermission(auth.PermViewDashboard, h) }
	mux.Handle("GET /api/v1/servers", view(s.handleListJSON))
	mux.Handle("GET /server/{id}", view(s.handleDetail))
	mux.Handle("GET /{$}", view(s.handleDashboard))
	return mux
}

// handleReport ingests one agent report.
func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	if !s.authOK(r) {
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
	if err := s.store.Save(&rep); err != nil {
		http.Error(w, "store error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// authOK checks the bearer token in constant time to avoid timing leaks.
func (s *Server) authOK(r *http.Request) bool {
	if s.token == "" {
		return true
	}
	got := []byte(r.Header.Get("Authorization"))
	want := []byte("Bearer " + s.token)
	return subtle.ConstantTimeCompare(got, want) == 1
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

func percent(used, total uint64) int {
	if total == 0 {
		return 0
	}
	return int(float64(used) / float64(total) * 100)
}
