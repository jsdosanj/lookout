//go:build windows

package software

import (
	"context"
	"strings"
)

// gather lists installed products on Windows via allow-listed argv-only WMIC.
//
// TODO(wave0): `wmic product` triggers an MSI consistency check and can be slow
// / miss user-scope & non-MSI installs. The robust path is reading the
// Uninstall registry hives (HKLM + HKCU, 32/64-bit) directly via
// golang.org/x/sys/windows/registry. Deferred — that's a new dependency the
// integrator must approve (see INTEGRATION-NOTES.md). WMIC gives a correct,
// dependency-free wave0 baseline.
func (c *Collector) gather(ctx context.Context) []Package {
	if c.runner == nil {
		return nil
	}
	out, err := c.runner.Run(ctx, "wmic", "product", "get",
		"Name,Version,Vendor", "/format:csv")
	if err != nil {
		return nil
	}
	return parseWMICProducts(out)
}

// parseWMICProducts parses `wmic product ... /format:csv`. The CSV header is
// "Node,Name,Vendor,Version"; column order is alphabetical regardless of the
// requested order, so we map by header name.
func parseWMICProducts(out string) []Package {
	lines := splitNonEmpty(out)
	if len(lines) < 2 {
		return nil
	}
	headers := strings.Split(lines[0], ",")
	col := func(name string) int {
		for i, h := range headers {
			if strings.EqualFold(strings.TrimSpace(h), name) {
				return i
			}
		}
		return -1
	}
	ni, vi, di := col("Name"), col("Version"), col("Vendor")
	var pkgs []Package
	for _, line := range lines[1:] {
		f := strings.Split(line, ",")
		get := func(idx int) string {
			if idx >= 0 && idx < len(f) {
				return strings.TrimSpace(f[idx])
			}
			return ""
		}
		name := get(ni)
		if name == "" {
			continue
		}
		pkgs = append(pkgs, Package{
			Name:    name,
			Version: get(vi),
			Vendor:  get(di),
			Source:  "registry",
		})
	}
	return pkgs
}

func splitNonEmpty(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			out = append(out, t)
		}
	}
	return out
}
