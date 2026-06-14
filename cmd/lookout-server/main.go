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
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jsdosanj/lookout/internal/alert"
	"github.com/jsdosanj/lookout/internal/auth"
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
	// Build the alert engine: webhook channels from LOOKOUT_ALERT_WEBHOOKS
	// (comma-separated, each SSRF-validated) plus a default fleet rule that fires
	// at warning+ with flap-damping and a 30-minute escalation reminder.
	recorder := alert.NewRecorder(50)
	eng := buildAlertEngine(recorder)

	// Background session GC: sweep expired sessions until shutdown (clean exit).
	stopGC := make(chan struct{})
	users.StartSessionGC(10*time.Minute, stopGC)
	defer close(stopGC)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           server.New(st, agents, token, requireAgent, a, eng, recorder).Routes(),
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

// buildAlertEngine wires the alert engine from environment configuration:
//   - LOOKOUT_ALERT_WEBHOOKS: comma-separated Slack/Teams/generic webhook URLs.
//     Each is SSRF-validated; an unsafe or unreachable URL is logged and skipped.
//   - LOOKOUT_ALERT_EMAIL: comma-separated recipient addresses (channel is
//     configured but SMTP delivery is not yet wired — see internal/alert).
//
// When any channel is configured, a default "fleet" rule fires on every server
// at warning severity or above, with 2-observation flap-damping and a 30-minute
// escalation reminder for unresolved incidents.
func buildAlertEngine(rec *alert.Recorder) *alert.Engine {
	var channels []alert.Channel
	var channelIDs []string

	i := 0
	for _, u := range strings.Split(os.Getenv("LOOKOUT_ALERT_WEBHOOKS"), ",") {
		if u = strings.TrimSpace(u); u == "" {
			continue
		}
		id := "webhook"
		if i > 0 {
			id = "webhook-" + strconv.Itoa(i+1)
		}
		ch, err := alert.NewWebhookChannel(id, u)
		if err != nil {
			log.Printf("alert webhook rejected (%s): %v", id, err)
			continue
		}
		channels = append(channels, ch)
		channelIDs = append(channelIDs, id)
		i++
	}

	var emailTo []string
	for _, e := range strings.Split(os.Getenv("LOOKOUT_ALERT_EMAIL"), ",") {
		if e = strings.TrimSpace(e); e != "" {
			emailTo = append(emailTo, e)
		}
	}
	if len(emailTo) > 0 {
		// SMTP creds not yet wired: channel is registered so rules/UI reference it,
		// but Send returns an explicit not-configured error (no fake success).
		channels = append(channels, alert.NewEmailChannel("email", emailTo, nil))
		channelIDs = append(channelIDs, "email")
	}

	if len(channels) == 0 {
		log.Print("NOTE: no alert channels configured — set LOOKOUT_ALERT_WEBHOOKS to enable alerting")
		return nil
	}

	rules := []alert.Rule{{
		ID:          "fleet-default",
		Name:        "Fleet: warning and above",
		Server:      "*",
		MinSeverity: alert.SevWarning,
		Channels:    channelIDs,
		FlapWindow:  2,
		RepeatEvery: 30 * time.Minute,
	}}
	log.Printf("alerting enabled: %d channel(s), %d rule(s)", len(channels), len(rules))
	return alert.NewEngine(rules, channels, rec.Log)
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
