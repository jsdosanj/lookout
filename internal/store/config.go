package store

import "time"

// Thresholds is one effective set of health limits used by Evaluate. A zero value
// for any field means "inherit" — from a group override, then the global defaults
// — and is never treated as an active limit, because 0% (or load 0) is not a
// meaningful threshold. This lets an override set only the fields it cares about
// without restating the rest.
type Thresholds struct {
	DiskWarnPct     float64       `json:"disk_warn_pct,omitempty"`
	DiskCritPct     float64       `json:"disk_crit_pct,omitempty"`
	MemWarnPct      float64       `json:"mem_warn_pct,omitempty"`
	MemCritPct      float64       `json:"mem_crit_pct,omitempty"`
	CPUWarnPct      float64       `json:"cpu_warn_pct,omitempty"`
	CPUCritPct      float64       `json:"cpu_crit_pct,omitempty"`
	LoadWarnPerCore float64       `json:"load_warn_per_core,omitempty"`
	LoadCritPerCore float64       `json:"load_crit_per_core,omitempty"`
	StaleAfter      time.Duration `json:"stale_after,omitempty"`
	// WatchServices names services that must be running on the host; a watched
	// service that is stopped or absent is critical. nil means inherit; a non-nil
	// (even empty) slice replaces the inherited list.
	WatchServices []string `json:"watch_services,omitempty"`
}

// DefaultThresholds are the out-of-the-box limits. Disk and memory match the
// historical hardcoded behavior exactly (disk 80/90, memory warns at 90 with no
// critical); CPU and load are the Phase-1 additions. StaleAfter is StaleAfter.
func DefaultThresholds() Thresholds {
	return Thresholds{
		DiskWarnPct: 80, DiskCritPct: 90,
		MemWarnPct: 90, // MemCritPct stays 0 (disabled) to preserve prior behavior
		CPUWarnPct: 85, CPUCritPct: 95,
		LoadWarnPerCore: 1.0, LoadCritPerCore: 2.0,
		StaleAfter: StaleAfter,
	}
}

// HealthConfig holds the global default thresholds plus optional per-host and
// per-group overrides. It is persisted by the store and is the replacement for
// the formerly-hardcoded threshold constants. The zero value is not valid; build
// one with DefaultHealthConfig.
type HealthConfig struct {
	Defaults  Thresholds            `json:"defaults"`
	Groups    map[string]Thresholds `json:"groups,omitempty"`     // group name -> override
	Hosts     map[string]Thresholds `json:"hosts,omitempty"`      // server ID -> override
	HostGroup map[string]string     `json:"host_group,omitempty"` // server ID -> group name
}

// DefaultHealthConfig returns a config with default thresholds and no overrides.
func DefaultHealthConfig() *HealthConfig {
	return &HealthConfig{Defaults: DefaultThresholds()}
}

// For resolves the effective thresholds for a server: package defaults, then the
// configured global defaults, then the host's group override (if any), then the
// host override — each layer filling only the fields it sets. A nil config (or a
// config with empty defaults) still yields safe package defaults.
func (c *HealthConfig) For(serverID string) Thresholds {
	t := DefaultThresholds()
	if c == nil {
		return t
	}
	t = merge(t, c.Defaults)
	if group, ok := c.HostGroup[serverID]; ok {
		if o, ok := c.Groups[group]; ok {
			t = merge(t, o)
		}
	}
	if o, ok := c.Hosts[serverID]; ok {
		t = merge(t, o)
	}
	return t
}

// merge returns base with override's non-zero fields applied. A zero field in the
// override means "keep the inherited value" (see Thresholds).
func merge(base, o Thresholds) Thresholds {
	if o.DiskWarnPct != 0 {
		base.DiskWarnPct = o.DiskWarnPct
	}
	if o.DiskCritPct != 0 {
		base.DiskCritPct = o.DiskCritPct
	}
	if o.MemWarnPct != 0 {
		base.MemWarnPct = o.MemWarnPct
	}
	if o.MemCritPct != 0 {
		base.MemCritPct = o.MemCritPct
	}
	if o.CPUWarnPct != 0 {
		base.CPUWarnPct = o.CPUWarnPct
	}
	if o.CPUCritPct != 0 {
		base.CPUCritPct = o.CPUCritPct
	}
	if o.LoadWarnPerCore != 0 {
		base.LoadWarnPerCore = o.LoadWarnPerCore
	}
	if o.LoadCritPerCore != 0 {
		base.LoadCritPerCore = o.LoadCritPerCore
	}
	if o.StaleAfter != 0 {
		base.StaleAfter = o.StaleAfter
	}
	if o.WatchServices != nil {
		base.WatchServices = o.WatchServices
	}
	return base
}
