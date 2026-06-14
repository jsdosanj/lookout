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
	ruleData := flag.String("rule-data", "lookout-rules.json", "path to the alert-rules file")
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
	// (comma-separated, each SSRF-validated) plus persisted, dashboard-editable
	// rules seeded with a fleet warning+ rule (flap-damping + 30-minute reminder).
	recorder := alert.NewRecorder(50)
	eng, rules := buildAlertEngine(recorder, *ruleData)

	// Background session GC: sweep expired sessions until shutdown (clean exit).
	stopGC := make(chan struct{})
	users.StartSessionGC(10*time.Minute, stopGC)
	defer close(stopGC)

	ctrl := server.New(st, agents, token, requireAgent, a, eng, recorder, rules)

	// Stale-host sweeper: re-evaluate the fleet on a cadence so a host that goes
	// silent (and turns "stale" after store.StaleAfter) actually fires an alert.
	// Without this, alerts only fire on report ingest and silent hosts never alert.
	stopSweep := make(chan struct{})
	ctrl.StartSweeper(time.Minute, stopSweep)
	defer close(stopSweep)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           ctrl.Routes(),
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
//   - LOOKOUT_ALERT_EMAIL: comma-separated recipient addresses. Live delivery
//     goes through the shared notification service (LOOKOUT_NOTIFY_SERVICE_URL +
//     LOOKOUT_NOTIFY_SERVICE_TOKEN, the POST /notify/send platform contract); if
//     the service isn't configured it falls back to the local EmailChannel, whose
//     send is an explicit not-configured error rather than a fake success.
//
// Rules are loaded from ruleData (seeded with a fleet warning+ rule on first run)
// and are editable from the dashboard.
func buildAlertEngine(rec *alert.Recorder, ruleData string) (*alert.Engine, *alert.RuleStore) {
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
		svcURL := strings.TrimSpace(os.Getenv("LOOKOUT_NOTIFY_SERVICE_URL"))
		svcTok := strings.TrimSpace(os.Getenv("LOOKOUT_NOTIFY_SERVICE_TOKEN"))
		if svcURL != "" && svcTok != "" {
			// Production transport: deliver email through the shared notification
			// service (it owns the provider key, dedupe, retry, and audit log).
			if ch, err := alert.NewNotifyServiceChannel("email", svcURL, svcTok, emailTo); err != nil {
				log.Printf("alert email via notify service rejected: %v", err)
			} else {
				channels = append(channels, ch)
				channelIDs = append(channelIDs, "email")
				log.Print("alert email: live delivery via shared notification service")
			}
		}
		if !containsID(channelIDs, "email") {
			// Fallback: register the local channel so rules/UI reference "email";
			// its Send returns an explicit not-configured error (no fake success).
			channels = append(channels, alert.NewEmailChannel("email", emailTo, nil))
			channelIDs = append(channelIDs, "email")
			log.Print("NOTE: alert email recipients set but notify service not configured — set LOOKOUT_NOTIFY_SERVICE_URL/_TOKEN for live delivery")
		}
	}

	if len(channels) == 0 {
		log.Print("NOTE: no alert channels configured — set LOOKOUT_ALERT_WEBHOOKS to enable alerting")
		return nil, nil
	}

	rules, err := alert.OpenRuleStore(ruleData, channelIDs)
	if err != nil {
		log.Fatalf("open rule store: %v", err)
	}
	ruleSet := rules.Rules()
	log.Printf("alerting enabled: %d channel(s), %d rule(s)", len(channels), len(ruleSet))
	return alert.NewEngine(ruleSet, channels, rec.Log), rules
}

// containsID reports whether id is already in ids.
func containsID(ids []string, id string) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
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
