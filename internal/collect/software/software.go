// Package software is collector C2: installed software + versions.
//
// Emits lookout.software.v1, consumed by Cairn, Ledger (license/SaaS
// reconciliation), Perimeter (version→CVE join) and Sightline (approved-software
// control). Cross-platform: package managers / app inventories are read via
// allow-listed argv-only commands; output is parsed in pure Go.
package software

import (
	"context"
	"runtime"
	"sort"
	"time"

	"github.com/jsdosanj/lookout/internal/collect/exec"
	"github.com/jsdosanj/lookout/internal/collector"
)

// SchemaID is the canonical evidence type for this collector.
const SchemaID = "lookout.software.v1"

// Software is the lookout.software.v1 payload.
type Software struct {
	Packages []Package `json:"packages"`
	Count    int       `json:"count"`
}

// Package is one installed package / application.
type Package struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Vendor  string `json:"vendor,omitempty"`
	Source  string `json:"source,omitempty"` // dpkg, rpm, brew, app, registry, ...
	Arch    string `json:"arch,omitempty"`
}

// Collector is C2.
type Collector struct{ runner *exec.Runner }

// New builds the software collector with a safe-exec runner.
func New(runner *exec.Runner) *Collector { return &Collector{runner: runner} }

// DefaultAllow lists the binaries this collector may invoke, per OS.
func DefaultAllow() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"system_profiler", "brew"}
	case "linux":
		return []string{"dpkg-query", "rpm"}
	case "windows":
		return []string{"wmic"}
	}
	return nil
}

// Meta implements collector.Collector.
func (c *Collector) Meta() collector.Metadata {
	return collector.Metadata{
		ID:              "software",
		SchemaID:        SchemaID,
		RequiredCaps:    []collector.Capability{"read.software"},
		DefaultInterval: 12 * time.Hour,
	}
}

// Collect gathers the installed-software list. The OS-specific gatherer
// (build-tagged) returns packages; here we de-duplicate, sort, and wrap.
func (c *Collector) Collect(ctx context.Context) (collector.Record, error) {
	pkgs := c.gather(ctx)
	pkgs = dedupe(pkgs)
	sort.Slice(pkgs, func(i, j int) bool {
		if pkgs[i].Name != pkgs[j].Name {
			return pkgs[i].Name < pkgs[j].Name
		}
		return pkgs[i].Version < pkgs[j].Version
	})
	sw := Software{Packages: pkgs, Count: len(pkgs)}
	return collector.Record{SchemaID: SchemaID, Payload: sw}, nil
}

// dedupe removes exact name+version+source duplicates (e.g. a package reported
// by two sources) while preserving the first occurrence.
func dedupe(in []Package) []Package {
	seen := make(map[string]struct{}, len(in))
	out := in[:0]
	for _, p := range in {
		if p.Name == "" {
			continue
		}
		key := p.Name + "\x00" + p.Version + "\x00" + p.Source
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, p)
	}
	return out
}
