//go:build darwin

package inventory

import (
	"context"
	"strconv"
	"strings"
)

// osDetail enriches the inventory with macOS hardware/OS facts via allow-listed
// argv-only commands. Every step is best-effort.
func (c *Collector) osDetail(ctx context.Context, inv *Inventory) {
	if c.runner == nil {
		return
	}
	// OS version + build from sw_vers.
	if v, err := c.runner.Run(ctx, "sw_vers", "-productVersion"); err == nil {
		inv.OS.Version = strings.TrimSpace(v)
	}
	if b, err := c.runner.Run(ctx, "sw_vers", "-buildVersion"); err == nil {
		inv.OS.Build = strings.TrimSpace(b)
	}

	// CPU model + RAM via sysctl (single keys, argv only).
	if m, err := c.runner.Run(ctx, "sysctl", "-n", "machdep.cpu.brand_string"); err == nil {
		inv.CPU.Model = strings.TrimSpace(m)
	}
	if r, err := c.runner.Run(ctx, "sysctl", "-n", "hw.memsize"); err == nil {
		if n, e := strconv.ParseUint(strings.TrimSpace(r), 10, 64); e == nil {
			inv.RAMBytes = n
		}
	}

	// Hardware model/serial via system_profiler (parse key: value lines).
	if out, err := c.runner.Run(ctx, "system_profiler", "SPHardwareDataType"); err == nil {
		parseDarwinHardware(out, inv)
	}
	inv.Manufacturer = "Apple"
}

// parseDarwinHardware extracts model, serial, and uptime from
// `system_profiler SPHardwareDataType` "key: value" output.
func parseDarwinHardware(out string, inv *Inventory) {
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		switch k {
		case "Model Name", "Model Identifier":
			if inv.Model == "" {
				inv.Model = v
			}
		case "Serial Number (system)":
			inv.Serial = v
		}
	}
}
