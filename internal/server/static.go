package server

import (
	"html/template"
	"os"
	"path/filepath"
	"time"

	"github.com/jsdosanj/lookout/internal/auth"
	"github.com/jsdosanj/lookout/internal/integrations"
	"github.com/jsdosanj/lookout/internal/store"
)

// demoAccount is the sample signed-in user shown in the static demo so the menu
// bar, account bar, and admin nav all render.
func demoPage(active string) pageView {
	return pageView{
		Active: active, Chrome: true, Static: true,
		UserEmail: "admin@lookout.app", UserRole: "owner", CanManageUsers: true,
	}
}

// WriteStaticDemo renders a frozen, self-contained copy of the whole dashboard
// (overview, per-server detail, the content pages, and the admin console) into
// dir with relative links, so the demo on GitHub Pages shows the full UI.
func WriteStaticDemo(dir string, servers []*store.Server, now time.Time) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	// Overview.
	dash := buildDashView(servers, now, true, nil)
	dash.UserEmail, dash.UserRole, dash.CanManageUsers = "admin@lookout.app", "owner", true
	if err := writeFile(filepath.Join(dir, "index.html"), dashboardTmpl, dash); err != nil {
		return err
	}
	// Per-server detail.
	for _, srv := range servers {
		dv := buildDetailView(srv, now, true, nil)
		dv.UserEmail, dv.UserRole, dv.CanManageUsers = "admin@lookout.app", "owner", true
		if err := writeFile(filepath.Join(dir, "server-"+safeID(srv.ID)+".html"), detailTmpl, dv); err != nil {
			return err
		}
	}

	// Content pages.
	var groups []intGroup
	for _, c := range integrations.Categories {
		if items := integrations.ByCategory(c.Key); len(items) > 0 {
			groups = append(groups, intGroup{Title: c.Title, Items: items})
		}
	}
	pages := []struct {
		file string
		tmpl *template.Template
		data any
	}{
		{"guides.html", guidesTmpl, demoPage("guides")},
		{"settings.html", settingsTmpl, demoPage("settings")},
		{"integrations.html", integrationsTmpl, intView{pageView: demoPage("integrations"), Groups: groups}},
		{"notifications.html", notificationsTmpl, notifyView{pageView: demoPage("notifications"), Items: integrations.ByCategory(integrations.NotificationCategory)}},
	}
	for _, p := range pages {
		if err := writeFile(filepath.Join(dir, p.file), p.tmpl, p.data); err != nil {
			return err
		}
	}
	// Per-connector detail pages.
	for _, in := range integrations.Catalog() {
		active := "integrations"
		if in.Category == integrations.NotificationCategory {
			active = "notifications"
		}
		if err := writeFile(filepath.Join(dir, "integration-"+in.ID+".html"), integrationDetailTmpl, intDetailView{pageView: demoPage(active), I: in}); err != nil {
			return err
		}
	}

	// Admin console (users / groups / departments / locations) with sample data.
	if err := auth.WriteStaticAdmin(dir); err != nil {
		return err
	}

	// .nojekyll lets GitHub Pages serve the files as-is.
	return os.WriteFile(filepath.Join(dir, ".nojekyll"), nil, 0o644)
}

func writeFile(path string, t *template.Template, data any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return t.Execute(f, data)
}
