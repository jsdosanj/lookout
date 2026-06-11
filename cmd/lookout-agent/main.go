// Command lookout-agent collects a host inventory + health report.
//
// This is the Phase-1 agent: it gathers the report and prints it as JSON. The
// secure transport that ships the report to the Lookout control plane is the
// next increment (see IMPLEMENTATION_PLAN.md).
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/jsdosanj/servmonitor/internal/collect"
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
	case "version", "-v", "--version":
		fmt.Println("lookout-agent", version)
	default:
		usage()
		os.Exit(2)
	}
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
  lookout-agent report     Collect and print this host's report as JSON
  lookout-agent version    Print the agent version
`)
}
