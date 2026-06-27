// Command lookout-server is the Lookout control plane: it ingests agent reports
// and serves the dashboard (behind login + RBAC).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jsdosanj/lookout/internal/alert"
	"github.com/jsdosanj/lookout/internal/auth"
	"github.com/jsdosanj/lookout/internal/check"
	"github.com/jsdosanj/lookout/internal/plugin"
	"github.com/jsdosanj/lookout/internal/server"
	"github.com/jsdosanj/lookout/internal/store"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	data := flag.String("data", "lookout.db", "path to the embedded SQLite database")
	authData := flag.String("auth-data", "lookout-users.json", "path to the users/sessions file")
	agentData := flag.String("agent-data", "lookout-agents.json", "path to the per-agent credentials file")
	ruleData := flag.String("rule-data", "lookout-rules.json", "path to the alert-rules file")
	healthCfg := flag.String("health-config", "", "optional path to a JSON health-config file (per-host/group thresholds + watched services); applied and persisted on boot")
	checksData := flag.String("checks", "", "optional path to a JSON file of TCP/HTTP checks to run as alert conditions")
	pluginsData := flag.String("plugins", "", "optional path to a JSON file of Nagios-style custom-check plugins")
	flag.Parse()

	token := os.Getenv("LOOKOUT_TOKEN")                                // shared agent enrollment token
	requireAgent := os.Getenv("LOOKOUT_REQUIRE_AGENT_TOKEN") == "true" // lock down legacy shared token
	secureCookies := os.Getenv("LOOKOUT_SECURE_COOKIES") == "true"     // set true behind TLS

	// Open the embedded SQLite store, migrating a pre-SQLite JSON data file on
	// first boot so existing deployments keep their inventory and history.
	_, statErr := os.Stat(*data)
	freshDB := os.IsNotExist(statErr)
	st, err := store.Open(*data)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()
	if freshDB {
		legacy := filepath.Join(filepath.Dir(*data), "lookout-data.json")
		if b, rerr := os.ReadFile(legacy); rerr == nil {
			if ierr := st.ImportLegacyJSON(b); ierr != nil {
				log.Printf("legacy data migration from %s failed: %v", legacy, ierr)
			} else {
				log.Printf("migrated legacy data from %s into %s", legacy, *data)
			}
		}
	}
	if r := strings.TrimSpace(os.Getenv("LOOKOUT_HISTORY_RETENTION")); r != "" {
		if d, perr := time.ParseDuration(r); perr != nil {
			log.Printf("ignoring invalid LOOKOUT_HISTORY_RETENTION %q: %v", r, perr)
		} else {
			st.SetRetention(d)
			log.Printf("sample retention set to %s", d)
		}
	}
	if *healthCfg != "" {
		if err := applyHealthConfig(st, *healthCfg); err != nil {
			log.Fatalf("health config: %v", err)
		}
		log.Printf("health config loaded from %s", *healthCfg)
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
	if eng != nil {
		// Persist acknowledgements in the store so a restart doesn't re-page an
		// operator for an incident they already acked.
		eng.SetAckStore(ackAdapter{st})
	}

	// Background session GC: sweep expired sessions until shutdown (clean exit).
	stopGC := make(chan struct{})
	users.StartSessionGC(10*time.Minute, stopGC)
	defer close(stopGC)

	ctrl := server.New(st, agents, token, requireAgent, a, eng, recorder, rules)

	// Load port/HTTP checks and custom-check plugins (optional). These run on a
	// cadence and feed the same health → alert pipeline as host reports.
	if *checksData != "" {
		var checks []check.Check
		if err := loadJSON(*checksData, &checks); err != nil {
			log.Fatalf("load checks: %v", err)
		}
		ctrl.SetChecks(checks)
		log.Printf("loaded %d check(s) from %s", len(checks), *checksData)
	}
	if *pluginsData != "" {
		var plugins []plugin.Plugin
		if err := loadJSON(*pluginsData, &plugins); err != nil {
			log.Fatalf("load plugins: %v", err)
		}
		ctrl.SetPlugins(plugins)
		log.Printf("loaded %d plugin(s) from %s", len(plugins), *pluginsData)
	}

	// Stale-host sweeper: re-evaluate the fleet on a cadence so a host that goes
	// silent (and turns "stale" after store.StaleAfter) actually fires an alert.
	// Without this, alerts only fire on report ingest and silent hosts never alert.
	stopSweep := make(chan struct{})
	ctrl.StartSweeper(time.Minute, stopSweep)
	defer close(stopSweep)

	// Check and plugin runners: probe targets / run plugins on the same cadence.
	stopChecks := make(chan struct{})
	ctrl.StartCheckRunner(time.Minute, stopChecks)
	defer close(stopChecks)
	stopPlugins := make(chan struct{})
	ctrl.StartPluginRunner(time.Minute, stopPlugins)
	defer close(stopPlugins)

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

// ackAdapter lets the SQLite store satisfy alert.AckStore. SaveAck/DeleteAck are
// promoted from the embedded *store.Store; only LoadAcks needs a type translation
// (so the store stays decoupled from the alert package).
type ackAdapter struct{ *store.Store }

func (a ackAdapter) LoadAcks() ([]alert.AckRecord, error) {
	rows, err := a.Store.Acks()
	if err != nil {
		return nil, err
	}
	out := make([]alert.AckRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, alert.AckRecord{RuleID: r.RuleID, Server: r.Server, Until: r.Until})
	}
	return out, nil
}

// applyHealthConfig loads a JSON HealthConfig from path and persists it as the
// active configuration (per-host/group thresholds + watched services).
func applyHealthConfig(st *store.Store, path string) error {
	var cfg store.HealthConfig
	if err := loadJSON(path, &cfg); err != nil {
		return err
	}
	return st.SetHealthConfig(&cfg)
}

// loadJSON reads and decodes a JSON file into v.
func loadJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
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
