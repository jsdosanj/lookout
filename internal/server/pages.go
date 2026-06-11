package server

import (
	"net/http"

	"github.com/jsdosanj/lookout/internal/auth"
)

// pageView is the data for the simple content pages (they only need the shell).
type pageView struct {
	Active         string
	Chrome         bool
	UserEmail      string
	CanManageUsers bool
}

func (s *Server) page(active string, r *http.Request) pageView {
	pv := pageView{Active: active, Chrome: true}
	if u := auth.CurrentUser(r); u != nil {
		pv.UserEmail = u.Email
		pv.CanManageUsers = u.Role.Can(auth.PermManageUsers)
	}
	return pv
}

func (s *Server) handleGuides(w http.ResponseWriter, r *http.Request) {
	render(w, guidesTmpl, s.page("guides", r))
}
func (s *Server) handleIntegrations(w http.ResponseWriter, r *http.Request) {
	render(w, integrationsTmpl, s.page("integrations", r))
}
func (s *Server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	render(w, notificationsTmpl, s.page("notifications", r))
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
  <p class="intro">Connect Lookout to the tools you already run. Each integration needs its own API credentials — add them to enable it. (Wiring is provided; you supply the keys.)</p>
  <h2>Security posture</h2>
  <div class="cards">
    <div class="icard"><span class="tag soon">Configure</span><h4>Sightline</h4><p>Pull your live security &amp; compliance posture into Lookout — see which servers meet NIST / HIPAA / SOC 2 controls.</p></div>
  </div>
  <h2 style="margin-top:1.4rem">Ticketing</h2>
  <div class="cards">
    <div class="icard"><span class="tag soon">Configure</span><h4>Jira</h4><p>Open a Jira issue automatically when an alert fires.</p></div>
    <div class="icard"><span class="tag soon">Configure</span><h4>ServiceNow</h4><p>Create a ServiceNow incident from an alert.</p></div>
    <div class="icard"><span class="tag soon">Configure</span><h4>Asana</h4><p>File an Asana task for the on-call owner.</p></div>
    <div class="icard"><span class="tag soon">Configure</span><h4>Trello</h4><p>Add a Trello card to your ops board.</p></div>
  </div>
  <h2 style="margin-top:1.4rem">Devices &amp; identity</h2>
  <div class="cards">
    <div class="icard"><span class="tag soon">Configure</span><h4>Jamf</h4><p>Enrich macOS hosts with Jamf MDM inventory and compliance.</p></div>
    <div class="icard"><span class="tag soon">Configure</span><h4>Microsoft Intune</h4><p>Pull device compliance and config from Intune.</p></div>
    <div class="icard"><span class="tag soon">Configure</span><h4>Kandji</h4><p>Apple device management signals from Kandji.</p></div>
    <div class="icard"><span class="tag soon">Configure</span><h4>JumpCloud</h4><p>Directory + device posture from JumpCloud.</p></div>
    <div class="icard"><span class="tag soon">Configure</span><h4>Active Directory</h4><p>Resolve users, groups, and machines from AD (LDAP).</p></div>
  </div>`)

var notificationsTmpl = mustPage("notifications", "Notifications", `
  <h1>Notifications</h1>
  <p class="intro">Get told the moment a server drifts out of health. Add a channel below — Slack, Teams, and webhooks just need an incoming-webhook URL; email needs SMTP; SMS needs a provider like Twilio.</p>
  <div class="cards">
    <div class="icard"><span class="tag soon">Add webhook URL</span><h4>Slack</h4><p>Post alerts to a Slack channel via an incoming webhook.</p></div>
    <div class="icard"><span class="tag soon">Add webhook URL</span><h4>Microsoft Teams</h4><p>Post alerts to a Teams channel via an incoming webhook.</p></div>
    <div class="icard"><span class="tag soon">Add URL</span><h4>Webhook</h4><p>POST every alert as JSON to any endpoint (PagerDuty, Opsgenie, your own handler).</p></div>
    <div class="icard"><span class="tag soon">Configure SMTP</span><h4>Email</h4><p>Email a plain-English summary of what changed and what to do.</p></div>
    <div class="icard"><span class="tag soon">Configure provider</span><h4>SMS / Text</h4><p>Text the on-call person on critical alerts (via Twilio or similar).</p></div>
  </div>
  <p class="intro" style="margin-top:1.2rem">The alerting engine that drives these is on the roadmap; the dashboard computes health today.</p>`)

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
