package server

import (
	"html/template"
	"os"
	"path/filepath"
	"time"

	"github.com/jsdosanj/servmonitor/internal/store"
)

// WriteStaticDemo renders a frozen, self-contained copy of the dashboard into
// dir (index.html + server-<id>.html), using relative links so it can be hosted
// on GitHub Pages. It is the live demo's generator and reuses the live templates.
func WriteStaticDemo(dir string, servers []*store.Server, now time.Time) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(dir, "index.html"), dashboardTmpl, buildDashView(servers, now, true)); err != nil {
		return err
	}
	for _, srv := range servers {
		name := "server-" + safeID(srv.ID) + ".html"
		if err := writeFile(filepath.Join(dir, name), detailTmpl, buildDetailView(srv, now, true)); err != nil {
			return err
		}
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
