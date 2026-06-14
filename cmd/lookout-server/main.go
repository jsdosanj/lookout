// Command lookout-server is the Lookout control plane: it ingests agent reports
// and serves the dashboard (behind login + RBAC).
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jsdosanj/lookout/internal/auth"
	"github.com/jsdosanj/lookout/internal/notify"
	"github.com/jsdosanj/lookout/internal/server"
	"github.com/jsdosanj/lookout/internal/store"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	data := flag.String("data", "lookout-data.json", "path to the server data file")
	authData := flag.String("auth-data", "lookout-users.json", "path to the users/sessions file")
	agentData := flag.String("agent-data", "lookout-agents.json", "path to the per-agent credentials file")
	flag.Parse()

	token := os.Getenv("LOOKOUT_TOKEN")                                // shared agent enrollment token
	requireAgent := os.Getenv("LOOKOUT_REQUIRE_AGENT_TOKEN") == "true" // lock down legacy shared token
	secureCookies := os.Getenv("LOOKOUT_SECURE_COOKIES") == "true"     // set true behind TLS

	st, err := store.Open(*data)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	agents, err := store.OpenAgents(*agentData)
	if err != nil {
		log.Fatalf("open agent store: %v", err)
	}
	users, err := auth.Open(*authData)
	if err != nil {
		log.Fatalf("open auth store: %v", err)
	}
	bootstrapOwner(users)

	a := auth.New(users, secureCookies, "Lookout")
	// Alert webhooks (Slack/Teams/generic) from LOOKOUT_ALERT_WEBHOOKS (comma-separated).
	var webhooks []string
	for _, u := range strings.Split(os.Getenv("LOOKOUT_ALERT_WEBHOOKS"), ",") {
		if u = strings.TrimSpace(u); u != "" {
			webhooks = append(webhooks, u)
		}
	}
	n := notify.New(webhooks)

	// Background session GC: sweep expired sessions until shutdown (clean exit).
	stopGC := make(chan struct{})
	users.StartSessionGC(10*time.Minute, stopGC)
	defer close(stopGC)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           server.New(st, agents, token, requireAgent, a, n).Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("Lookout control plane on %s (data: %s)", *addr, *data)
	if token == "" {
		log.Print("WARNING: LOOKOUT_TOKEN not set — agent reports are unauthenticated (dev only)")
	}
	if requireAgent {
		log.Print("NOTE: LOOKOUT_REQUIRE_AGENT_TOKEN=true — legacy shared token is rejected on /report (per-agent tokens only)")
	}
	if !secureCookies {
		log.Print("NOTE: LOOKOUT_SECURE_COOKIES != 'true' — cookies not marked Secure (run behind TLS in production)")
	}

	// Graceful shutdown on SIGINT/SIGTERM so the GC goroutine and in-flight
	// requests wind down cleanly.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("serve: %v", err)
		}
	}()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

// bootstrapOwner creates the first owner account on a fresh install from
// LOOKOUT_ADMIN_EMAIL + LOOKOUT_ADMIN_PASSWORD.
func bootstrapOwner(users *auth.Store) {
	if users.Count() > 0 {
		return
	}
	email, pw := os.Getenv("LOOKOUT_ADMIN_EMAIL"), os.Getenv("LOOKOUT_ADMIN_PASSWORD")
	if email == "" || pw == "" {
		log.Print("NOTE: no users yet. Set LOOKOUT_ADMIN_EMAIL and LOOKOUT_ADMIN_PASSWORD to create the first owner.")
		return
	}
	if _, err := users.CreateUser(email, "Owner", auth.RoleOwner, pw); err != nil {
		log.Printf("could not create owner: %v", err)
		return
	}
	log.Printf("created owner account %s", email)
}
