//go:build darwin

package software

import (
	"context"
	"strings"
)

// gather lists macOS applications via system_profiler (JSON-free text parse)
// and Homebrew formulae if brew is present. All argv-only, no shell.
func (c *Collector) gather(ctx context.Context) []Package {
	if c.runner == nil {
		return nil
	}
	var pkgs []Package

	// Installed applications: `system_profiler SPApplicationsDataType` emits
	// indented "key: value" blocks; each app has a name line then Version line.
	if out, err := c.runner.Run(ctx, "system_profiler", "SPApplicationsDataType"); err == nil {
		pkgs = append(pkgs, parseDarwinApps(out)...)
	}

	// Homebrew formulae (optional).
	if c.runner.Allowed("brew") {
		if out, err := c.runner.Run(ctx, "brew", "list", "--versions"); err == nil {
			pkgs = append(pkgs, parseBrew(out)...)
		}
	}
	return pkgs
}

// parseDarwinApps parses SPApplicationsDataType text. An app block starts with a
// line ending in ":" at 4-space indent (the app name) followed by deeper-
// indented "Version:" / "Obtained from:" lines.
func parseDarwinApps(out string) []Package {
	var pkgs []Package
	var cur *Package
	flush := func() {
		if cur != nil && cur.Name != "" {
			pkgs = append(pkgs, *cur)
		}
		cur = nil
	}
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimRight(raw, " ")
		if line == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		trimmed := strings.TrimSpace(line)
		// App-name header: 4-space indent, ends with ":" and no value after.
		if indent == 4 && strings.HasSuffix(trimmed, ":") {
			flush()
			cur = &Package{Name: strings.TrimSuffix(trimmed, ":"), Source: "app"}
			continue
		}
		if cur == nil {
			continue
		}
		if k, v, ok := strings.Cut(trimmed, ":"); ok {
			v = strings.TrimSpace(v)
			switch strings.TrimSpace(k) {
			case "Version":
				cur.Version = v
			case "Obtained from":
				cur.Vendor = v
			}
		}
	}
	flush()
	return pkgs
}

// parseBrew parses `brew list --versions` lines: "name ver1 ver2".
func parseBrew(out string) []Package {
	var pkgs []Package
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		p := Package{Name: f[0], Source: "brew", Vendor: "Homebrew"}
		if len(f) > 1 {
			p.Version = f[1]
		}
		pkgs = append(pkgs, p)
	}
	return pkgs
}
