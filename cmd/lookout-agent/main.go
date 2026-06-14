// Command lookout-agent collects a host inventory + health report.
//
// This is the Phase-1 agent: it gathers the report and prints it as JSON. The
// secure transport that ships the report to the Lookout control plane is the
// next increment (see IMPLEMENTATION_PLAN.md).
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/jsdosanj/lookout/internal/agent"
	"github.com/jsdosanj/lookout/internal/collect"
	"github.com/jsdosanj/lookout/internal/collect/envelope"
	"github.com/jsdosanj/lookout/internal/collect/identity"
	"github.com/jsdosanj/lookout/internal/collect/scheduler"
	"github.com/jsdosanj/lookout/internal/collect/shipper"
	"github.com/jsdosanj/lookout/internal/collect/spool"
	"github.com/jsdosanj/lookout/internal/collector"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "0.1.0-dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "report":
		if err := report(); err != nil {
			fmt.Fprintln(os.Stderr, "lookout-agent:", err)
			os.Exit(1)
		}
	case "run":
		if err := run(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "lookout-agent:", err)
			os.Exit(1)
		}
	case "collect":
		if err := collectCmd(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "lookout-agent:", err)
			os.Exit(1)
		}
	case "version", "-v", "--version":
		fmt.Println("lookout-agent", version)
	default:
		usage()
		os.Exit(2)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	server := fs.String("server", "", "control plane URL, e.g. http://monitor.example.com:8080")
	token := fs.String("token", "", "shared enrollment token (or set LOOKOUT_TOKEN)")
	tokenFile := fs.String("token-file", defaultAgentTokenFile(), "path to the per-agent token (created on first enroll)")
	interval := fs.Duration("interval", time.Minute, "how often to report")
	once := fs.Bool("once", false, "report once and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *token == "" {
		*token = os.Getenv("LOOKOUT_TOKEN")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Per-agent token, migration-safe: reuse the stored token if present,
	// otherwise enroll once with the shared token and persist the result. If
	// enrollment isn't possible (no shared token / older control plane), fall
	// back to reporting with the shared token (legacy behavior).
	reportToken := *token
	if t, err := loadAgentToken(*tokenFile); err == nil && t != "" {
		reportToken = t
	} else if *token != "" {
		hostname, _ := os.Hostname()
		if _, at, err := agent.Enroll(ctx, *server, *token, hostname); err == nil {
			if err := saveAgentToken(*tokenFile, at); err != nil {
				fmt.Fprintln(os.Stderr, "lookout-agent: could not persist agent token:", err)
			}
			reportToken = at
		} else {
			fmt.Fprintln(os.Stderr, "lookout-agent: enroll failed, using shared token:", err)
		}
	}

	return agent.Run(ctx, agent.Config{ServerURL: *server, Token: reportToken, Interval: *interval, Once: *once})
}

// defaultAgentTokenFile picks a per-user path for the persisted per-agent token.
func defaultAgentTokenFile() string {
	return filepath.Join(defaultStateDir(), "agent-token")
}

func loadAgentToken(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func saveAgentToken(path, token string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(token+"\n"), 0o600)
}

// collectCmd drives the Universal Collector pipeline (Wave-0 reference
// collectors C1/C2/C4): load-or-enroll a signed agent identity, register the
// reference collectors, then schedule them and ship each signed envelope to the
// Keystone ingest control plane. This is the integration of internal/collect/*
// per its INTEGRATION-NOTES (the legacy `report`/`run` paths are unchanged).
//
// The ingest base URL is CONFIGURABLE (--ingest-url or LOOKOUT_INGEST_URL); the
// domain is never hardcoded.
func collectCmd(args []string) error {
	fs := flag.NewFlagSet("collect", flag.ExitOnError)
	ingestURL := fs.String("ingest-url", os.Getenv("LOOKOUT_INGEST_URL"), "Keystone ingest base URL, e.g. https://api.example.com")
	token := fs.String("token", os.Getenv("LOOKOUT_TOKEN"), "one-time enrollment token (first run only)")
	stateDir := fs.String("state-dir", defaultStateDir(), "directory for the agent identity + spool")
	once := fs.Bool("once", false, "run each collector once and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *ingestURL == "" {
		return errors.New("collect: --ingest-url (or LOOKOUT_INGEST_URL) is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 1. Load-or-enroll the agent identity.
	idPath := filepath.Join(*stateDir, "identity.json")
	id, err := identity.Load(idPath)
	if errors.Is(err, os.ErrNotExist) {
		id, err = enroll(ctx, *ingestURL, *token, idPath)
	}
	if err != nil {
		return fmt.Errorf("collect: identity: %w", err)
	}
	if !id.Enrolled() {
		return errors.New("collect: agent is not enrolled (provide --token on first run)")
	}

	// 2. Register the Wave-0 reference collectors into the shared registry.
	collect.RegisterReferenceCollectors()

	// 3. Open the spool (bounded; backs offline/retry).
	sp, err := spool.New(filepath.Join(*stateDir, "spool"), 64<<20)
	if err != nil {
		return fmt.Errorf("collect: spool: %w", err)
	}

	// 4. Build the shipper (outbound-only, redirect-disabled).
	ship := shipper.New(shipper.Config{IngestURL: *ingestURL}, id, sp)

	// 5. Sink: marshal payload -> build+sign envelope -> ship.
	sink := func(ctx context.Context, c collector.Collector, rec collector.Record) error {
		payloadBytes, err := envelope.MarshalPayload(rec)
		if err != nil {
			return err
		}
		env, err := envelope.Build(id, c.Meta().ID, rec, "", payloadBytes)
		if err != nil {
			return err
		}
		return ship.Ship(ctx, env)
	}

	// Fail-closed capabilities: a real deploy receives these from the signed
	// /v1/policy bundle. For a zero-config local/free install we grant exactly
	// the three read caps the reference collectors declare (note §capability).
	// TODO(wave0): fetch + verify the signed policy bundle from /v1/policy and
	// derive GrantedCaps from it instead of this static local default.
	granted := map[collector.Capability]bool{
		"read.inventory": true,
		"read.software":  true,
		"read.posture":   true,
	}
	policies := map[string]scheduler.Policy{}
	for _, c := range collector.Default().All() {
		policies[c.Meta().ID] = scheduler.Policy{Enabled: true, GrantedCaps: granted}
	}

	sched := scheduler.New(scheduler.Config{
		Registry: collector.Default(),
		Policies: policies,
		Sink:     sink,
		OnEvent: func(e scheduler.Event) {
			if e.Err != nil {
				fmt.Fprintf(os.Stderr, "collector %s: %v\n", e.CollectorID, e.Err)
			}
		},
	})

	if *once {
		for _, c := range collector.Default().All() {
			sched.RunOnce(ctx, c.Meta().ID)
		}
		return nil
	}

	// 6. Run until signalled.
	sched.Run(ctx)
	return nil
}

// enroll performs first-run enrollment: generate a keypair, POST a self-signed
// EnrollRequest to /v1/enroll, apply the assigned binding, and persist.
func enroll(ctx context.Context, ingestURL, token, idPath string) (*identity.Identity, error) {
	if token == "" {
		return nil, errors.New("enroll: --token is required on first run")
	}
	id, err := identity.Generate()
	if err != nil {
		return nil, err
	}
	hostname, _ := os.Hostname()
	req, sig, err := id.BuildEnrollRequest(token, hostname, runtime.GOOS, runtime.GOARCH, version)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, ingestURL+"/v1/enroll", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Signature", sig)

	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse // outbound-only: never follow redirects
		},
	}
	res, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return nil, fmt.Errorf("enroll: control plane returned %d: %s", res.StatusCode, bytes.TrimSpace(msg))
	}
	var resp identity.EnrollResponse
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("enroll: decode response: %w", err)
	}
	if err := id.Apply(resp); err != nil {
		return nil, err
	}
	if err := id.Save(idPath); err != nil {
		return nil, err
	}
	return id, nil
}

// defaultStateDir picks a per-user state directory for the agent identity+spool.
func defaultStateDir() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "lookout-agent")
	}
	return ".lookout-agent"
}

func report() error {
	rep, err := collect.Collect()
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}

func usage() {
	fmt.Fprint(os.Stderr, `Lookout agent

Usage:
  lookout-agent report                       Collect and print this host's report as JSON
  lookout-agent run --server URL [--token T] Report to the control plane (--once, --interval)
  lookout-agent collect --ingest-url URL     Universal Collector: enroll + ship signed evidence
                         [--token T] [--once]   (C1/C2/C4; --token on first run to enroll)
  lookout-agent version                      Print the agent version
`)
}
