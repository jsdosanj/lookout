package server

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jsdosanj/lookout/internal/auth"
	"github.com/jsdosanj/lookout/internal/collect"
	"github.com/jsdosanj/lookout/internal/store"
)

// ── view models ─────────────────────────────────────────────────────────────

type dashView struct {
	Servers                      []cardView
	Attention                    []cardView // warning/critical/stale, for the needs-attention panel
	OK, Warning, Critical, Stale int
	Total                        int
	EncryptedCount               int
	OSDistJSON                   template.JS
	Active                       string
	Chrome                       bool // render the side panel (off for the static demo)
	UserEmail                    string
	CanManageUsers               bool
}

type cardView struct {
	ID, Href, Platform, Version, CPU string
	Cores                            int
	Status                           string
	Reasons                          []string
	CPUPct                           int
	CPUBar                           template.CSS
	MemPct                           int
	MemBar                           template.CSS
	MemUsedMB, MemTotalMB            uint64
	TopDiskMount                     string
	TopDiskPct                       int
	Services                         int
	LastSeen                         string
}

type detailView struct {
	ID, BackHref                             string
	Active                                   string
	Chrome                                   bool
	UserEmail                                string
	CanManageUsers                           bool
	OS, Platform, Version, Kernel, Arch, CPU string
	Virtualization, Encryption               string
	Cores                                    int
	Status                                   string
	Reasons                                  []string
	Uptime                                   string
	CPUPct                                   int
	CPUBar                                   template.CSS
	MemPct                                   int
	MemBar                                   template.CSS
	MemUsedMB, MemTotalMB                    uint64
	LoadAvg                                  []float64
	Disks                                    []diskView
	Network                                  []collect.NetInterface
	Processes                                []collect.Process
	Services                                 []collect.Service
	RunningServices                          int
	Packages                                 int
	PackageList                              []collect.Package
	LastSeen, CollectedAt                    string
	ChartData                                template.JS
}

type diskView struct {
	Mount, FS       string
	UsedMB, TotalMB uint64
	Pct             int
	Bar             template.CSS
}

// ── handlers ─────────────────────────────────────────────────────────────────

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	dv := buildDashView(s.store.List(), time.Now().UTC(), false)
	dv.Active = "overview"
	if u := auth.CurrentUser(r); u != nil {
		dv.UserEmail = u.Email
		dv.CanManageUsers = u.Role.Can(auth.PermManageUsers)
	}
	render(w, dashboardTmpl, dv)
}

func (s *Server) handleDetail(w http.ResponseWriter, r *http.Request) {
	srv, ok := s.store.Get(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	dv := buildDetailView(srv, time.Now().UTC(), false)
	dv.Active = "overview"
	if u := auth.CurrentUser(r); u != nil {
		dv.UserEmail = u.Email
		dv.CanManageUsers = u.Role.Can(auth.PermManageUsers)
	}
	render(w, detailTmpl, dv)
}

// ── view builders (shared by live handlers and the static demo generator) ────

func buildDashView(servers []*store.Server, now time.Time, static bool) dashView {
	var d dashView
	d.Chrome = !static
	osCount := map[string]int{}
	for _, srv := range servers {
		h := store.Evaluate(srv, now)
		d.Total++
		plat := srv.LastReport.Host.Platform
		if plat == "" {
			plat = srv.LastReport.Host.OS
		}
		osCount[plat]++
		if srv.LastReport.Host.Encryption == "on" {
			d.EncryptedCount++
		}
		switch h.Status {
		case "ok":
			d.OK++
		case "warning":
			d.Warning++
		case "critical":
			d.Critical++
		case "stale":
			d.Stale++
		}
		rep := srv.LastReport
		card := cardView{
			ID:         srv.ID,
			Href:       serverHref(srv.ID, static),
			Platform:   rep.Host.Platform,
			Version:    rep.Host.Version,
			CPU:        rep.Specs.CPUModel,
			Cores:      rep.Specs.CPUCores,
			Status:     h.Status,
			Reasons:    h.Reasons,
			CPUPct:     int(rep.Specs.CPUPercent + 0.5),
			MemPct:     percent(rep.Specs.MemUsedMB, rep.Specs.MemTotalMB),
			MemUsedMB:  rep.Specs.MemUsedMB,
			MemTotalMB: rep.Specs.MemTotalMB,
			Services:   len(rep.Services),
			LastSeen:   humanAgo(now.Sub(srv.LastSeen)),
		}
		card.CPUBar = barStyle(card.CPUPct)
		card.MemBar = barStyle(card.MemPct)
		for _, dk := range rep.Specs.Disks {
			if p := percent(dk.UsedMB, dk.TotalMB); p >= card.TopDiskPct {
				card.TopDiskPct, card.TopDiskMount = p, dk.Mount
			}
		}
		d.Servers = append(d.Servers, card)
		if h.Status != "ok" {
			d.Attention = append(d.Attention, card)
		}
	}
	d.OSDistJSON = osDistJSON(osCount)
	return d
}

// osDistJSON marshals an OS→count map into {labels, counts} for a doughnut chart.
func osDistJSON(osCount map[string]int) template.JS {
	type od struct {
		Labels []string `json:"labels"`
		Counts []int    `json:"counts"`
	}
	keys := make([]string, 0, len(osCount))
	for k := range osCount {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return osCount[keys[i]] > osCount[keys[j]] })
	o := od{Labels: []string{}, Counts: []int{}}
	for _, k := range keys {
		o.Labels = append(o.Labels, k)
		o.Counts = append(o.Counts, osCount[k])
	}
	b, _ := json.Marshal(o)
	return template.JS(b)
}

func buildDetailView(srv *store.Server, now time.Time, static bool) detailView {
	h := store.Evaluate(srv, now)
	rep := srv.LastReport
	back := "/"
	if static {
		back = "index.html"
	}
	dv := detailView{
		ID:          srv.ID,
		BackHref:    back,
		Chrome:      !static,
		OS:          rep.Host.OS,
		Platform:    rep.Host.Platform,
		Version:     rep.Host.Version,
		Kernel:      rep.Host.Kernel,
		Arch:        rep.Host.Arch,
		CPU:            rep.Specs.CPUModel,
		Virtualization: rep.Host.Virtualization,
		Encryption:     rep.Host.Encryption,
		Cores:          rep.Specs.CPUCores,
		Status:         h.Status,
		Reasons:        h.Reasons,
		Uptime:         uptimeHuman(rep.Host.UptimeSeconds),
		CPUPct:         int(rep.Specs.CPUPercent + 0.5),
		MemPct:         percent(rep.Specs.MemUsedMB, rep.Specs.MemTotalMB),
		MemUsedMB:      rep.Specs.MemUsedMB,
		MemTotalMB:     rep.Specs.MemTotalMB,
		LoadAvg:        rep.Specs.LoadAvg,
		Network:        rep.Specs.Network,
		Processes:      rep.Specs.Processes,
		Packages:       len(rep.Packages),
		PackageList:    rep.Packages,
		Services:       rep.Services,
		LastSeen:       humanAgo(now.Sub(srv.LastSeen)),
		CollectedAt:    rep.CollectedAt.Format("2006-01-02 15:04 MST"),
		ChartData:      chartJSON(srv.History),
	}
	dv.CPUBar = barStyle(dv.CPUPct)
	dv.MemBar = barStyle(dv.MemPct)
	for _, dk := range rep.Specs.Disks {
		p := percent(dk.UsedMB, dk.TotalMB)
		dv.Disks = append(dv.Disks, diskView{Mount: dk.Mount, FS: dk.FS, UsedMB: dk.UsedMB, TotalMB: dk.TotalMB, Pct: p, Bar: barStyle(p)})
	}
	for _, sv := range rep.Services {
		if sv.Status == "running" {
			dv.RunningServices++
		}
	}
	return dv
}

// ── helpers ──────────────────────────────────────────────────────────────────

// chartJSON marshals a server's history into a JS object for Chart.js.
func chartJSON(history []store.Sample) template.JS {
	type cd struct {
		Labels []string  `json:"labels"`
		CPU    []float64 `json:"cpu"`
		Mem    []float64 `json:"mem"`
		Disk   []float64 `json:"disk"`
	}
	c := cd{Labels: []string{}, CPU: []float64{}, Mem: []float64{}, Disk: []float64{}}
	for _, s := range history {
		c.Labels = append(c.Labels, s.At.Local().Format("15:04"))
		c.CPU = append(c.CPU, s.CPU)
		c.Mem = append(c.Mem, s.Mem)
		c.Disk = append(c.Disk, s.Disk)
	}
	b, _ := json.Marshal(c)
	return template.JS(b)
}

func render(w http.ResponseWriter, t *template.Template, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// serverHref is the link to a server's detail page: a route in the live app, or
// a flat .html file in the static demo.
func serverHref(id string, static bool) string {
	if static {
		return "server-" + safeID(id) + ".html"
	}
	return "/server/" + id
}

// safeID makes an id usable as a filename.
func safeID(id string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, id)
}

func barStyle(pct int) template.CSS {
	if pct < 0 {
		pct = 0
	} else if pct > 100 {
		pct = 100
	}
	return template.CSS(fmt.Sprintf("width:%d%%", pct))
}

func humanAgo(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func uptimeHuman(sec int64) string {
	if sec <= 0 {
		return "—"
	}
	d, h, m := sec/86400, (sec%86400)/3600, (sec%3600)/60
	switch {
	case d > 0:
		return fmt.Sprintf("%dd %dh %dm", d, h, m)
	case h > 0:
		return fmt.Sprintf("%dh %dm", h, m)
	default:
		return fmt.Sprintf("%dm", m)
	}
}
