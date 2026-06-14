package server

import (
	"html/template"
	"net/http"

	"github.com/jsdosanj/lookout/internal/auth"
	"github.com/jsdosanj/lookout/internal/integrations"
)

// pageView is the data for the simple content pages (they only need the shell).
type pageView struct {
	Active         string
	Chrome         bool
	Static         bool
	UserEmail      string
	UserRole       string
	CanManageUsers bool
	CSRF           template.HTML
}

type intGroup struct {
	Title string
	Items []integrations.Integration
}
type intView struct {
	pageView
	Groups []intGroup
}
type notifyView struct {
	pageView
	Items []integrations.Integration
}
type intDetailView struct {
	pageView
	I integrations.Integration
}

func (s *Server) page(active string, r *http.Request) pageView {
	pv := pageView{Active: active, Chrome: true}
	if u := auth.CurrentUser(r); u != nil {
		pv.UserEmail = u.Email
		pv.UserRole = string(u.Role)
		pv.CanManageUsers = u.Role.Can(auth.PermManageUsers)
		pv.CSRF = csrfField(auth.CSRFToken(r))
	}
	return pv
}

func (s *Server) handleGuides(w http.ResponseWriter, r *http.Request) {
	render(w, guidesTmpl, s.page("guides", r))
}
func (s *Server) handleIntegrations(w http.ResponseWriter, r *http.Request) {
	var groups []intGroup
	for _, c := range integrations.Categories {
		if items := integrations.ByCategory(c.Key); len(items) > 0 {
			groups = append(groups, intGroup{Title: c.Title, Items: items})
		}
	}
	render(w, integrationsTmpl, intView{pageView: s.page("integrations", r), Groups: groups})
}
func (s *Server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	render(w, notificationsTmpl, notifyView{pageView: s.page("notifications", r), Items: integrations.ByCategory(integrations.NotificationCategory)})
}
func (s *Server) handleIntegrationDetail(w http.ResponseWriter, r *http.Request) {
	in, ok := integrations.ByID(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	active := "integrations"
	if in.Category == integrations.NotificationCategory {
		active = "notifications"
	}
	render(w, integrationDetailTmpl, intDetailView{pageView: s.page(active, r), I: in})
}
func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	render(w, settingsTmpl, s.page("settings", r))
}

// ── content ──────────────────────────────────────────────────────────────────

var guidesTmpl = mustPage("guides", "Help & Guides", `
  <h1>Help &amp; Guides</h1>
  <p class="intro">Short, plain-English explainers for everything on the dashboard. New to Lookout? Start at the top.</p>
  <div class="guide"><h4>Overview</h4><p>Every server you monitor, as a card, color-coded by health: <b>ok</b> (all good), <b>warning</b> (something's trending the wrong way), <b>critical</b> (act now), <b>stale</b> (the agent stopped reporting — the box may be down).</p></div>
  <div class="guide"><h4>How health is decided</h4><p>A disk over 80% used is a warning, over 90% is critical. Memory over 90% is a warning. No report for 5 minutes is stale. The worst signal wins, and the reason is always spelled out — e.g. "disk /data is 94% full."</p></div>
  <div class="guide"><h4>Performance charts</h4><p>Open any server to see CPU, memory, and disk usage over time. Watch for sustained climbs — they're your early warning before something fills up or falls over.</p></div>
  <div class="guide"><h4>OS distribution &amp; encryption</h4><p>The Overview shows the mix of operating systems across your fleet and how many machines have disk encryption (FileVault / BitLocker / LUKS) turned on — a quick compliance read.</p></div>
  <div class="guide"><h4>Integrations</h4><p>Connect Lookout to the tools you already use — ticketing (Jira, ServiceNow, …), MDM (Jamf, Intune, …), Active Directory, and Sightline for security posture. Each needs its own credentials; see the Integrations tab.</p></div>
  <div class="guide"><h4>Notifications</h4><p>Get told when something breaks — Slack, Teams, email, SMS, or a webhook. Configure channels in the Notifications tab.</p></div>
  <div class="guide"><h4>Users, roles &amp; MFA</h4><p>Admins add users at <b>Users</b> and assign roles (owner / admin / operator / viewer). Everyone can turn on two-factor authentication from <b>Account</b>.</p></div>
  <div class="guide"><h4>Installing agents</h4><p>Run one small agent per server (Linux, Windows, macOS). It only makes outbound connections — no open ports. See the docs for per-OS install steps.</p></div>`)

var integrationsTmpl = mustPage("integrations", "Integrations", `
  <h1>Integrations</h1>
  <p class="intro">Connect Lookout to the tools you already run. Connectors marked <b>In development</b> are coming soon; click any to see what it does and the credentials it'll need.</p>
  {{range .Groups}}
  <h2>{{.Title}}</h2>
  <div class="cards">
    {{range .Items}}
    <a class="icard" href="{{if $.Static}}integration-{{.ID}}.html{{else}}/integrations/{{.ID}}{{end}}"><span class="tag {{.Status.Tag}}">{{.Status.Label}}</span><h4>{{.Name}}</h4><p>{{.Description}}</p></a>
    {{end}}
  </div>
  {{end}}`)

var notificationsTmpl = mustPage("notifications", "Notifications", `
  <h1>Notifications</h1>
  <p class="intro">Lookout alerts you the moment a server worsens into <b>warning</b> or <b>critical</b>. Slack, Teams, and generic webhooks are live today; email and SMS are in development.</p>
  <div class="cards">
    {{range .Items}}
    <a class="icard" href="{{if $.Static}}integration-{{.ID}}.html{{else}}/integrations/{{.ID}}{{end}}"><span class="tag {{.Status.Tag}}">{{.Status.Label}}</span><h4>{{.Name}}</h4><p>{{.Description}}</p></a>
    {{end}}
  </div>
  <div class="guide" style="margin-top:1.2rem"><h4>Enable webhook alerts (today)</h4><p>Set <code>LOOKOUT_ALERT_WEBHOOKS</code> on the control plane to one or more incoming-webhook URLs (comma-separated). Slack and Teams both accept the format Lookout sends.</p></div>`)

var integrationDetailTmpl = mustPage("integration", "Integration", `
  <a class="back" href="{{if .Static}}{{if eq .I.Category "notifications"}}notifications.html{{else}}integrations.html{{end}}{{else}}/{{if eq .I.Category "notifications"}}notifications{{else}}integrations{{end}}{{end}}">&larr; Back</a>
  <div style="display:flex;align-items:center;gap:.8rem;flex-wrap:wrap"><h1>{{.I.Name}}</h1><span class="tag {{.I.Status.Tag}}">{{.I.Status.Label}}</span></div>
  <p class="intro">{{.I.Description}}</p>
  {{if eq .I.Status.Tag "live"}}
  <div class="guide"><h4>Enable it</h4><p>{{.I.EnableHint}}</p></div>
  {{else}}
  <div class="guide"><h4>Coming soon</h4><p>This connector is in development. When it ships, you'll configure it here with:</p>
    <ul style="margin:.6rem 0 0 1.2rem;color:var(--muted)">{{range .I.Needs}}<li>{{.}}</li>{{end}}</ul></div>
  {{end}}`)

var settingsTmpl = mustPage("settings", "Settings", `
  <h1>Settings</h1>
  <p class="intro">Customize how the dashboard looks and which widgets appear on the Overview. Settings are saved in your browser.</p>
  <h2>Appearance</h2>
  <div class="toggle-row"><span>Theme</span><button class="linkbtn" onclick="lkTheme()" style="color:var(--brand);font-weight:700">◑ Toggle light / dark</button></div>
  <h2 style="margin-top:1.2rem">Overview widgets</h2>
  <div class="toggle-row"><label for="w-os">Operating-systems chart</label><input type="checkbox" id="w-os" style="width:auto"></div>
  <div class="toggle-row"><label for="w-enc">Disk-encryption summary</label><input type="checkbox" id="w-enc" style="width:auto"></div>
  <script>
    (function(){
      function init(id,key){var el=document.getElementById(id);el.checked=localStorage.getItem(key)!=='0';
        el.addEventListener('change',function(){localStorage.setItem(key,el.checked?'1':'0')});}
      init('w-os','lk-show-os'); init('w-enc','lk-show-enc');
    })();
  </script>`)
