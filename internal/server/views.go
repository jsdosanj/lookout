package server

import (
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/jsdosanj/servmonitor/internal/collect"
	"github.com/jsdosanj/servmonitor/internal/store"
)

// ── view models ─────────────────────────────────────────────────────────────

type dashView struct {
	Servers                      []cardView
	OK, Warning, Critical, Stale int
	Total                        int
}

type cardView struct {
	ID, Href, Platform, Version, CPU string
	Cores                            int
	Status                           string
	Reasons                          []string
	MemPct                           int
	MemBar                           template.CSS
	MemUsedMB, MemTotalMB            uint64
	TopDiskMount                     string
	TopDiskPct                       int
	Services                         int
	LastSeen                         string
}

type detailView struct {
	ID, BackHref                                 string
	OS, Platform, Version, Kernel, Arch, CPU     string
	Cores                                        int
	Status                                       string
	Reasons                                      []string
	Uptime                                       string
	MemPct                                       int
	MemBar                                       template.CSS
	MemUsedMB, MemTotalMB                        uint64
	LoadAvg                                      []float64
	Disks                                        []diskView
	Services                                     []collect.Service
	RunningServices                              int
	Packages                                     int
	LastSeen, CollectedAt                        string
}

type diskView struct {
	Mount, FS       string
	UsedMB, TotalMB uint64
	Pct             int
	Bar             template.CSS
}

// ── handlers ─────────────────────────────────────────────────────────────────

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	render(w, dashboardTmpl, buildDashView(s.store.List(), time.Now().UTC(), false))
}

func (s *Server) handleDetail(w http.ResponseWriter, r *http.Request) {
	srv, ok := s.store.Get(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	render(w, detailTmpl, buildDetailView(srv, time.Now().UTC(), false))
}

// ── view builders (shared by live handlers and the static demo generator) ────

func buildDashView(servers []*store.Server, now time.Time, static bool) dashView {
	var d dashView
	for _, srv := range servers {
		h := store.Evaluate(srv, now)
		d.Total++
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
			MemPct:     percent(rep.Specs.MemUsedMB, rep.Specs.MemTotalMB),
			MemUsedMB:  rep.Specs.MemUsedMB,
			MemTotalMB: rep.Specs.MemTotalMB,
			Services:   len(rep.Services),
			LastSeen:   humanAgo(now.Sub(srv.LastSeen)),
		}
		card.MemBar = barStyle(card.MemPct)
		for _, dk := range rep.Specs.Disks {
			if p := percent(dk.UsedMB, dk.TotalMB); p >= card.TopDiskPct {
				card.TopDiskPct, card.TopDiskMount = p, dk.Mount
			}
		}
		d.Servers = append(d.Servers, card)
	}
	return d
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
		OS:          rep.Host.OS,
		Platform:    rep.Host.Platform,
		Version:     rep.Host.Version,
		Kernel:      rep.Host.Kernel,
		Arch:        rep.Host.Arch,
		CPU:         rep.Specs.CPUModel,
		Cores:       rep.Specs.CPUCores,
		Status:      h.Status,
		Reasons:     h.Reasons,
		Uptime:      uptimeHuman(rep.Host.UptimeSeconds),
		MemPct:      percent(rep.Specs.MemUsedMB, rep.Specs.MemTotalMB),
		MemUsedMB:   rep.Specs.MemUsedMB,
		MemTotalMB:  rep.Specs.MemTotalMB,
		LoadAvg:     rep.Specs.LoadAvg,
		Packages:    len(rep.Packages),
		Services:    rep.Services,
		LastSeen:    humanAgo(now.Sub(srv.LastSeen)),
		CollectedAt: rep.CollectedAt.Format("2006-01-02 15:04 MST"),
	}
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
