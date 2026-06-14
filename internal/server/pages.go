package server

import (
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jsdosanj/lookout/internal/alert"
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
	Items           []integrations.Integration
	AlertingEnabled bool
	CanManageAlerts bool
	Editable        bool // rules are persisted and can be edited from the dashboard
	Rules           []alert.Rule
	Channels        []string
	Incidents       []alert.OpenIncident
	Activity        []alert.Activity
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
	v := notifyView{
		pageView:        s.page("notifications", r),
		Items:           integrations.ByCategory(integrations.NotificationCategory),
		AlertingEnabled: s.alerts.Enabled(),
	}
	if u := auth.CurrentUser(r); u != nil {
		v.CanManageAlerts = u.Role.Can(auth.PermManageAlerts)
	}
	// Active rules and recent deliveries are only shown to users who manage alerts.
	if v.CanManageAlerts {
		v.Rules = s.alerts.Rules()
		v.Channels = s.alerts.ChannelIDs()
		v.Incidents = s.alerts.OpenIncidents()
		v.Editable = s.rules != nil
		if s.activity != nil {
			v.Activity = s.activity.Recent(20)
		}
	}
	render(w, notificationsTmpl, v)
}

// handleRuleSave creates or updates a persisted alert rule from the dashboard
// form, then pushes the new rule set into the live engine. Behind PermManageAlerts.
func (s *Server) handleRuleSave(w http.ResponseWriter, r *http.Request) {
	if s.rules == nil {
		http.Error(w, "rule editing not available", http.StatusServiceUnavailable)
		return
	}
	rule := alert.Rule{
		ID:          r.FormValue("id"),
		Name:        strings.TrimSpace(r.FormValue("name")),
		Server:      strings.TrimSpace(r.FormValue("server")),
		MinSeverity: alert.SeverityOf(r.FormValue("min_severity")),
		Channels:    r.Form["channels"],
		FlapWindow:  atoiDefault(r.FormValue("flap_window"), 1),
		RepeatEvery: time.Duration(atoiDefault(r.FormValue("repeat_min"), 0)) * time.Minute,
	}
	if rule.Name == "" || len(rule.Channels) == 0 {
		http.Error(w, "rule needs a name and at least one channel", http.StatusBadRequest)
		return
	}
	if _, err := s.rules.Upsert(rule); err != nil {
		http.Error(w, "save failed", http.StatusInternalServerError)
		return
	}
	s.alerts.SetRules(s.rules.Rules())
	http.Redirect(w, r, "/notifications", http.StatusSeeOther)
}

// handleRuleDelete removes a persisted rule and refreshes the live engine.
func (s *Server) handleRuleDelete(w http.ResponseWriter, r *http.Request) {
	if s.rules == nil {
		http.Error(w, "rule editing not available", http.StatusServiceUnavailable)
		return
	}
	if err := s.rules.Delete(r.FormValue("id")); err != nil {
		http.Error(w, "delete failed", http.StatusInternalServerError)
		return
	}
	s.alerts.SetRules(s.rules.Rules())
	http.Redirect(w, r, "/notifications", http.StatusSeeOther)
}

// handleAck acknowledges (or snoozes) an open incident so its reminder cascade
// stops. snooze_min > 0 snoozes for that many minutes; 0 acks until the incident
// resolves or worsens. Behind PermManageAlerts.
func (s *Server) handleAck(w http.ResponseWriter, r *http.Request) {
	var until time.Time
	if m := atoiDefault(r.FormValue("snooze_min"), 0); m > 0 {
		until = time.Now().UTC().Add(time.Duration(m) * time.Minute)
	}
	s.alerts.Acknowledge(r.FormValue("rule_id"), r.FormValue("server"), until)
	http.Redirect(w, r, "/notifications", http.StatusSeeOther)
}

func atoiDefault(s string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
		return n
	}
	return def
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
  <p class="intro">Lookout alerts you the moment a server worsens into <b>warning</b> or <b>critical</b>, deduplicates ongoing problems, damps flapping, and re-notifies until the incident clears. Slack, Teams, and generic webhooks are live today; email and SMS are in development.</p>
  {{if .CanManageAlerts}}
  <div class="guide" style="margin-bottom:1.2rem">
    <h4>Alerting status</h4>
    {{if .AlertingEnabled}}<p><span class="tag live">Active</span> Rules below are live and delivering.</p>
    {{else}}<p><span class="tag soon">Off</span> No alert channels configured. Set <code>LOOKOUT_ALERT_WEBHOOKS</code> to turn alerting on.</p>{{end}}
  </div>
  {{if .Incidents}}
  <h2>Open incidents</h2>
  <p class="intro">Acknowledge to stop the reminder cascade without waiting for recovery. A worsening severity re-alerts; recovery still sends the all-clear.</p>
  <table class="alert-table">
    <thead><tr><th>Server</th><th>Rule</th><th>Severity</th><th>Cause</th><th>State</th><th>Actions</th></tr></thead>
    <tbody>
      {{range .Incidents}}
      <tr><td>{{.Server}}</td><td>{{.RuleName}}</td><td>{{.Severity}}</td><td>{{.Reason}}</td>
        <td>{{if .Acked}}<span class="tag soon">acknowledged</span>{{else}}<span class="tag live">active</span>{{end}}</td>
        <td>
          <form method="post" action="/notifications/ack" style="display:inline">{{$.CSRF}}
            <input type="hidden" name="rule_id" value="{{.RuleID}}"><input type="hidden" name="server" value="{{.Server}}">
            <button class="linkbtn" type="submit" style="color:var(--brand);font-weight:700">Acknowledge</button></form>
          <form method="post" action="/notifications/ack" style="display:inline;margin-left:.6rem">{{$.CSRF}}
            <input type="hidden" name="rule_id" value="{{.RuleID}}"><input type="hidden" name="server" value="{{.Server}}">
            <input type="hidden" name="snooze_min" value="60">
            <button class="linkbtn" type="submit" style="color:var(--muted);font-weight:700">Snooze 1h</button></form>
        </td></tr>
      {{end}}
    </tbody>
  </table>
  {{end}}
  {{if .Rules}}
  <h2>Active rules</h2>
  <table class="alert-table">
    <thead><tr><th>Rule</th><th>Server</th><th>Fires at</th><th>Flap window</th><th>Repeat</th><th>Channels</th>{{if .Editable}}<th></th>{{end}}</tr></thead>
    <tbody>
      {{range .Rules}}
      <tr><td>{{.Name}}</td><td>{{if or (eq .Server "") (eq .Server "*")}}all{{else}}{{.Server}}{{end}}</td>
        <td>{{.MinSeverity.String}}+</td><td>{{.FlapWindow}} obs</td>
        <td>{{if .RepeatEvery}}{{.RepeatEvery}}{{else}}—{{end}}</td>
        <td>{{range $i, $c := .Channels}}{{if $i}}, {{end}}{{$c}}{{end}}</td>
        {{if $.Editable}}<td><form method="post" action="/notifications/rules/delete" style="display:inline">{{$.CSRF}}
          <input type="hidden" name="id" value="{{.ID}}">
          <button class="linkbtn" type="submit" style="color:#c0392b;font-weight:700">Delete</button></form></td>{{end}}</tr>
      {{end}}
    </tbody>
  </table>
  {{end}}
  {{if .Editable}}
  <div class="guide" style="margin-bottom:1.2rem">
    <h4>Add or update a rule</h4>
    <form method="post" action="/notifications/rules/save" style="display:grid;gap:.6rem;max-width:32rem">{{.CSRF}}
      <label>Name <input name="name" required style="width:100%"></label>
      <label>Server (blank or <code>*</code> = all, else exact server id) <input name="server" placeholder="*" style="width:100%"></label>
      <label>Fire at
        <select name="min_severity">
          <option value="warning">warning and above</option>
          <option value="critical">critical and above</option>
          <option value="stale">stale only</option>
        </select></label>
      <label>Flap window (observations before acting) <input name="flap_window" type="number" min="1" value="2" style="width:6rem"></label>
      <label>Reminder cadence in minutes (0 = never) <input name="repeat_min" type="number" min="0" value="30" style="width:6rem"></label>
      <fieldset style="border:1px solid var(--line);border-radius:8px;padding:.6rem"><legend>Channels</legend>
        {{range .Channels}}<label style="display:block"><input type="checkbox" name="channels" value="{{.}}" style="width:auto"> {{.}}</label>{{end}}
      </fieldset>
      <button type="submit" class="linkbtn" style="color:var(--brand);font-weight:700;justify-self:start">Save rule</button>
    </form>
  </div>
  {{end}}
  <h2 style="margin-top:1.2rem">Recent alert activity</h2>
  {{if .Activity}}
  <table class="alert-table">
    <thead><tr><th>Time</th><th>Server</th><th>State</th><th>Channel</th><th>Result</th></tr></thead>
    <tbody>
      {{range .Activity}}
      <tr><td>{{.At.Format "Jan 2 15:04"}}</td><td>{{.Server}}</td>
        <td>{{if .Resolved}}resolved{{else}}{{.Severity}}{{if .Repeat}} (reminder){{end}}{{end}}</td>
        <td>{{.Channel}}</td>
        <td>{{if .Err}}<span class="tag soon">failed</span>{{else}}<span class="tag live">sent</span>{{end}}</td></tr>
      {{end}}
    </tbody>
  </table>
  {{else}}<p class="intro">No alerts delivered yet. When a server crosses a threshold, the delivery shows here.</p>{{end}}
  {{end}}
  <h2 style="margin-top:1.2rem">Channels</h2>
  <div class="cards">
    {{range .Items}}
    <a class="icard" href="{{if $.Static}}integration-{{.ID}}.html{{else}}/integrations/{{.ID}}{{end}}"><span class="tag {{.Status.Tag}}">{{.Status.Label}}</span><h4>{{.Name}}</h4><p>{{.Description}}</p></a>
    {{end}}
  </div>
  <div class="guide" style="margin-top:1.2rem"><h4>Enable webhook alerts (today)</h4><p>Set <code>LOOKOUT_ALERT_WEBHOOKS</code> on the control plane to one or more incoming-webhook URLs (comma-separated). Every URL is validated against an SSRF guard before any request is made. Slack and Teams both accept the format Lookout sends.</p></div>`)

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
