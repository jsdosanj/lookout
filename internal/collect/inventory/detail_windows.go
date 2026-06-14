//go:build windows

package inventory

import (
	"context"
	"strconv"
	"strings"
)

// osDetail enriches the inventory on Windows via allow-listed argv-only WMIC
// queries. WMIC is invoked with explicit args (no shell), and its CSV output is
// parsed in pure Go.
//
// TODO(wave0): WMIC is deprecated on newer Windows; add a PowerShell
// CIM-cmdlet path (still argv-only, no shell) as a fallback. For wave0 the
// degrade-to-empty behavior is acceptable.
func (c *Collector) osDetail(ctx context.Context, inv *Inventory) {
	if c.runner == nil {
		return
	}
	// Computer system: manufacturer, model, total RAM.
	if out, err := c.runner.Run(ctx, "wmic", "computersystem", "get",
		"Manufacturer,Model,TotalPhysicalMemory,Domain", "/format:csv"); err == nil {
		parseWMICSystem(out, inv)
	}
	// BIOS serial.
	if out, err := c.runner.Run(ctx, "wmic", "bios", "get", "SerialNumber", "/format:csv"); err == nil {
		if v := wmicValue(out, "SerialNumber"); v != "" {
			inv.Serial = v
		}
	}
	// CPU name.
	if out, err := c.runner.Run(ctx, "wmic", "cpu", "get", "Name", "/format:csv"); err == nil {
		if v := wmicValue(out, "Name"); v != "" {
			inv.CPU.Model = v
		}
	}
	// OS version + build.
	if out, err := c.runner.Run(ctx, "wmic", "os", "get", "Version,BuildNumber", "/format:csv"); err == nil {
		inv.OS.Version = wmicValue(out, "Version")
		inv.OS.Build = wmicValue(out, "BuildNumber")
	}
}

// parseWMICSystem maps the computersystem CSV row into the inventory.
func parseWMICSystem(out string, inv *Inventory) {
	inv.Manufacturer = wmicValue(out, "Manufacturer")
	inv.Model = wmicValue(out, "Model")
	inv.Domain = wmicValue(out, "Domain")
	if v := wmicValue(out, "TotalPhysicalMemory"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			inv.RAMBytes = n
		}
	}
}

// wmicValue parses WMIC "/format:csv" output (header row + data rows, the first
// column is "Node") and returns the value under the named column from the first
// data row. Returns "" if absent.
func wmicValue(out, col string) string {
	lines := splitNonEmpty(out)
	if len(lines) < 2 {
		return ""
	}
	headers := strings.Split(lines[0], ",")
	idx := -1
	for i, h := range headers {
		if strings.EqualFold(strings.TrimSpace(h), col) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ""
	}
	fields := strings.Split(lines[1], ",")
	if idx >= len(fields) {
		return ""
	}
	return strings.TrimSpace(fields[idx])
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
