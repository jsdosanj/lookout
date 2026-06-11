// Command lookout-agent collects a host inventory + health report.
//
// This is the Phase-1 agent: it gathers the report and prints it as JSON. The
// secure transport that ships the report to the Lookout control plane is the
// next increment (see IMPLEMENTATION_PLAN.md).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jsdosanj/lookout/internal/agent"
	"github.com/jsdosanj/lookout/internal/collect"
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
	token := fs.String("token", "", "enrollment token (or set LOOKOUT_TOKEN)")
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
	return agent.Run(ctx, agent.Config{ServerURL: *server, Token: *token, Interval: *interval, Once: *once})
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
  lookout-agent version                      Print the agent version
`)
}
