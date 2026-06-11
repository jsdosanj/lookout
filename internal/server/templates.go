package server

import "html/template"

var funcs = template.FuncMap{
	"barclass": func(pct int) string {
		switch {
		case pct >= 90:
			return "crit"
		case pct >= 80:
			return "warn"
		default:
			return ""
		}
	},
}

const cssConst = `
:root{--bg:#0b1220;--panel:#121a2e;--line:#1f2a44;--ink:#e6edf7;--muted:#94a3b8;--brand:#6366f1;--ok:#22c55e;--warn:#f59e0b;--crit:#ef4444;--stale:#64748b}
body.light{--bg:#f1f5f9;--panel:#ffffff;--line:#e2e8f0;--ink:#0f172a;--muted:#64748b}
.overview-row{display:grid;grid-template-columns:1.4fr 1fr;gap:1rem;margin-bottom:1.4rem}
.panel{background:var(--panel);border:1px solid var(--line);border-radius:14px;padding:1.1rem}
.panel-h{margin:0 0 .6rem;color:var(--muted);text-transform:uppercase;letter-spacing:.05em;font-size:.78rem}
.panel.enc{display:flex;flex-direction:column;justify-content:center}
.enc-big{font-size:2.4rem;font-weight:800;color:var(--ink)}
.stats{display:grid;grid-template-columns:repeat(5,1fr);gap:.8rem;margin-bottom:1.4rem}
.stat{background:var(--panel);border:1px solid var(--line);border-radius:14px;padding:1rem 1.2rem}
.stat .n{font-size:2rem;font-weight:800;color:var(--ink);line-height:1.1}
.stat .l{font-size:.76rem;color:var(--muted);text-transform:uppercase;letter-spacing:.05em;margin-top:.2rem}
.attention{background:var(--panel);border:1px solid var(--line);border-radius:14px;padding:1rem 1.2rem;margin-bottom:1.4rem}
.att-row{display:flex;align-items:center;gap:.7rem;padding:.5rem 0;border-top:1px solid var(--line)}
.att-row:first-of-type{border-top:none}
.att-row b{color:var(--ink)}
@media(max-width:760px){.overview-row{grid-template-columns:1fr}.stats{grid-template-columns:repeat(2,1fr)}}
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:ui-sans-serif,system-ui,-apple-system,'Segoe UI',sans-serif;background:var(--bg);color:var(--ink);line-height:1.5}
a{color:inherit;text-decoration:none}
code{font-family:ui-monospace,Menlo,monospace;background:#0b1220;border:1px solid var(--line);border-radius:6px;padding:.15rem .4rem;font-size:.85em}
.wrap{max-width:1100px;margin:0 auto;padding:1.5rem}
header.top{display:flex;align-items:center;justify-content:space-between;border-bottom:1px solid var(--line);padding:1rem 1.5rem}
.topnav{display:flex;gap:1.2rem;align-items:center;font-size:.88rem}
.topnav a{color:var(--muted)}.topnav a:hover{color:#fff}
.linkbtn{background:none;border:none;color:var(--muted);cursor:pointer;font:inherit;padding:0}.linkbtn:hover{color:#fff}
.logo{font-weight:800;font-size:1.2rem;letter-spacing:-.02em}
.logo b{color:var(--brand)}
.summary{display:flex;gap:.6rem;flex-wrap:wrap;margin-bottom:1.4rem}
.chip{background:var(--panel);border:1px solid var(--line);border-radius:999px;padding:.35rem .9rem;font-size:.85rem;color:var(--muted)}
.chip b{color:var(--ink)}
.grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(300px,1fr));gap:1rem}
.card{background:var(--panel);border:1px solid var(--line);border-radius:14px;padding:1.1rem;display:block;transition:border-color .15s,transform .15s}
.card:hover{border-color:#33406b;transform:translateY(-2px)}
.card-head{display:flex;align-items:flex-start;justify-content:space-between;gap:.5rem;margin-bottom:.4rem}
.host{font-weight:700;font-size:1.05rem}
.sub{color:var(--muted);font-size:.82rem}
.pill{font-size:.72rem;font-weight:700;padding:.2rem .55rem;border-radius:999px;text-transform:uppercase;letter-spacing:.04em;white-space:nowrap}
.s-ok{background:rgba(34,197,94,.15);color:var(--ok)}
.s-warning{background:rgba(245,158,11,.15);color:var(--warn)}
.s-critical{background:rgba(239,68,68,.15);color:var(--crit)}
.s-stale{background:rgba(100,116,139,.18);color:var(--stale)}
.dot{width:9px;height:9px;border-radius:50%;display:inline-block;margin-right:.35rem}
.d-ok{background:var(--ok)}.d-warning{background:var(--warn)}.d-critical{background:var(--crit)}.d-stale{background:var(--stale)}
.metric{display:flex;justify-content:space-between;font-size:.85rem;color:var(--muted);margin-top:.5rem}
.bar{height:7px;background:#0b1220;border:1px solid var(--line);border-radius:5px;overflow:hidden;margin-top:.25rem}
.bar i{display:block;height:100%;background:var(--brand)}
.bar i.warn{background:var(--warn)}.bar i.crit{background:var(--crit)}
.reasons{margin-top:.6rem;font-size:.8rem;color:var(--warn)}
.reasons.crit{color:var(--crit)}
.empty{background:var(--panel);border:1px dashed var(--line);border-radius:14px;padding:2.5rem;text-align:center;color:var(--muted)}
h1{font-size:1.5rem;margin-bottom:.3rem}
h2{margin:1.4rem 0 .6rem;color:var(--muted);text-transform:uppercase;letter-spacing:.05em;font-size:.8rem}
.back{color:var(--muted);font-size:.85rem;display:inline-block;margin-bottom:.8rem}
table{width:100%;border-collapse:collapse;font-size:.88rem;background:var(--panel);border:1px solid var(--line);border-radius:12px;overflow:hidden}
th,td{text-align:left;padding:.55rem .8rem;border-bottom:1px solid var(--line)}
th{color:var(--muted);font-weight:600;font-size:.78rem;text-transform:uppercase;letter-spacing:.04em}
tr:last-child td{border-bottom:none}
.kv{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:.8rem}
.kv .item{background:var(--panel);border:1px solid var(--line);border-radius:12px;padding:.9rem}
.kv .item .l{color:var(--muted);font-size:.75rem;text-transform:uppercase;letter-spacing:.04em}
.kv .item .v{font-weight:600;margin-top:.2rem;word-break:break-word}
.svc-scroll{max-height:340px;overflow:auto;border:1px solid var(--line);border-radius:12px;margin-top:.3rem}
.svc-scroll table{border:none}
footer{color:var(--muted);font-size:.8rem;text-align:center;padding:2rem}
/* side-panel app shell */
.app{display:flex;min-height:100vh}
.side{width:230px;flex:0 0 230px;border-right:1px solid var(--line);padding:1.2rem 1rem;display:flex;flex-direction:column;position:sticky;top:0;height:100vh;background:var(--panel)}
.brand{font-weight:800;font-size:1.25rem;color:var(--ink);padding:.2rem .5rem}.brand b{color:var(--brand)}
.sidenav{display:flex;flex-direction:column;gap:.15rem;margin-top:1.3rem}
.sidenav a{padding:.5rem .7rem;border-radius:8px;color:var(--muted);font-weight:600;font-size:.92rem}
.sidenav a:hover{color:var(--ink);background:rgba(127,127,140,.1)}
.sidenav a.on{background:rgba(99,102,241,.16);color:var(--ink)}
.side-foot{margin-top:auto;display:flex;flex-direction:column;gap:.5rem;font-size:.85rem;border-top:1px solid var(--line);padding-top:.9rem}
.side-foot a{color:var(--muted)}.side-foot a:hover{color:var(--ink)}
.content{flex:1;min-width:0;padding:1.6rem 2rem;max-width:1180px}
.intro{color:var(--muted);max-width:680px;margin:.2rem 0 1.4rem}
.cards{display:grid;grid-template-columns:repeat(auto-fill,minmax(260px,1fr));gap:1rem}
.icard{background:var(--panel);border:1px solid var(--line);border-radius:14px;padding:1.1rem}
.icard h4{margin:.1rem 0 .3rem}
.icard p{color:var(--muted);font-size:.88rem;margin:0 0 .8rem}
.tag{display:inline-block;font-size:.72rem;font-weight:700;padding:.15rem .5rem;border-radius:999px}
.tag.soon{background:rgba(245,158,11,.15);color:var(--warn)}
.tag.live{background:rgba(34,197,94,.15);color:var(--ok)}
.guide{background:var(--panel);border:1px solid var(--line);border-radius:12px;padding:1.1rem 1.3rem;margin-bottom:.9rem}
.guide h4{margin:0 0 .3rem}.guide p{color:var(--muted);margin:0;font-size:.92rem}
.toggle-row{display:flex;align-items:center;justify-content:space-between;background:var(--panel);border:1px solid var(--line);border-radius:12px;padding:.9rem 1.1rem;margin-bottom:.6rem;max-width:520px}
@media(max-width:820px){.app{flex-direction:column}.side{width:auto;flex:none;height:auto;position:static;flex-direction:row;flex-wrap:wrap;align-items:center;gap:.4rem}.sidenav{flex-direction:row;flex-wrap:wrap;margin:0}.side-foot{margin:0 0 0 auto;flex-direction:row;border:none;padding:0}.content{padding:1.2rem}}
`

const headOpen = `<!DOCTYPE html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">`
const styleTag = `<style>` + cssConst + `</style></head><body>` +
	`<script>function lkTheme(){document.body.classList.toggle('light');try{localStorage.setItem('lk-theme',document.body.classList.contains('light')?'light':'dark')}catch(e){}}` +
	`(function(){try{if(localStorage.getItem('lk-theme')==='light')document.body.classList.add('light')}catch(e){}})();</script>`

const dashBody = `
<div class="wrap">
  <h1 style="margin-bottom:1rem">Overview</h1>
  <div class="stats">
    <div class="stat"><div class="n">{{.Total}}</div><div class="l">Servers</div></div>
    <div class="stat"><div class="n" style="color:var(--ok)">{{.OK}}</div><div class="l">OK</div></div>
    <div class="stat"><div class="n" style="color:var(--warn)">{{.Warning}}</div><div class="l">Warning</div></div>
    <div class="stat"><div class="n" style="color:var(--crit)">{{.Critical}}</div><div class="l">Critical</div></div>
    <div class="stat"><div class="n" style="color:var(--stale)">{{.Stale}}</div><div class="l">Stale</div></div>
  </div>
  {{if .Attention}}
  <div class="attention">
    <h3 class="panel-h" style="color:var(--warn)">&#9888; Needs attention</h3>
    {{range .Attention}}<a class="att-row" href="{{.Href}}"><span class="pill s-{{.Status}}">{{.Status}}</span><b>{{.ID}}</b><span class="sub">{{range .Reasons}}{{.}} · {{end}}{{if not .Reasons}}{{.Platform}}{{end}}</span></a>{{end}}
  </div>
  {{end}}
  {{if .Servers}}
  <div class="overview-row" id="overviewRow">
    <div class="panel" id="panel-os"><h3 class="panel-h">Operating systems</h3><canvas id="osChart" height="150"></canvas></div>
    <div class="panel enc" id="panel-enc"><h3 class="panel-h">Disk encryption</h3><div class="enc-big">{{.EncryptedCount}}/{{.Total}}</div><p class="sub">servers with FileVault / BitLocker / LUKS enabled</p></div>
  </div>
  <div class="grid">
    {{range .Servers}}
    <a class="card" href="{{.Href}}">
      <div class="card-head">
        <div><div class="host">{{.ID}}</div><div class="sub">{{.Platform}} · {{.Version}}</div></div>
        <span class="pill s-{{.Status}}">{{.Status}}</span>
      </div>
      <div class="sub">{{.CPU}} · {{.Cores}} cores</div>
      <div class="metric"><span>CPU</span><span>{{.CPUPct}}%</span></div>
      <div class="bar"><i class="{{barclass .CPUPct}}" style="{{.CPUBar}}"></i></div>
      <div class="metric"><span>Memory</span><span>{{.MemPct}}% · {{.MemUsedMB}}/{{.MemTotalMB}} MB</span></div>
      <div class="bar"><i class="{{barclass .MemPct}}" style="{{.MemBar}}"></i></div>
      <div class="metric"><span>Top disk {{.TopDiskMount}}</span><span>{{.TopDiskPct}}%</span></div>
      <div class="metric"><span>Services tracked</span><span>{{.Services}}</span></div>
      {{if .Reasons}}<div class="reasons {{if eq .Status "critical"}}crit{{end}}">{{range .Reasons}}&#9888; {{.}}<br>{{end}}</div>{{end}}
      <div class="sub" style="margin-top:.5rem">Last seen {{.LastSeen}}</div>
    </a>
    {{end}}
  </div>
  {{else}}
  <div class="empty">No servers reporting yet.<br><br>Install the agent on a server and point it here:<br><br>
    <code>lookout-agent run --server http://THIS_HOST:8080 --token YOUR_TOKEN</code></div>
  {{end}}
</div>
<script src="https://cdn.jsdelivr.net/npm/chart.js@4"></script>
<script>
  const OS = {{.OSDistJSON}};
  if (window.Chart && document.getElementById('osChart') && OS.labels && OS.labels.length) {
    new Chart(document.getElementById('osChart'), {
      type: 'doughnut',
      data: { labels: OS.labels, datasets: [{ data: OS.counts, borderWidth: 0,
        backgroundColor: ['#6366f1','#22d3ee','#f59e0b','#22c55e','#ef4444','#a855f7','#64748b','#ec4899'] }] },
      options: { plugins: { legend: { position: 'right', labels: { color: '#94a3b8', boxWidth: 12 } } } }
    });
  }
</script>
<script>
  (function(){
    var os = localStorage.getItem('lk-show-os') === '0', enc = localStorage.getItem('lk-show-enc') === '0';
    if (os) { var a = document.getElementById('panel-os'); if (a) a.style.display = 'none'; }
    if (enc) { var b = document.getElementById('panel-enc'); if (b) b.style.display = 'none'; }
    var row = document.getElementById('overviewRow'); if (row && os && enc) row.style.display = 'none';
  })();
</script>
`

const detailBody = `
<div class="wrap">
  <a class="back" href="{{.BackHref}}">&larr; All servers</a>
  <div style="display:flex;align-items:center;gap:.8rem;flex-wrap:wrap">
    <h1>{{.ID}}</h1><span class="pill s-{{.Status}}">{{.Status}}</span>
  </div>
  <div class="sub">{{.Platform}} &middot; {{.Version}} &middot; {{.Arch}} &middot; kernel {{.Kernel}}</div>
  {{if .Reasons}}<div class="reasons {{if eq .Status "critical"}}crit{{end}}" style="margin-top:.6rem">{{range .Reasons}}&#9888; {{.}}<br>{{end}}</div>{{end}}

  <h2>Overview</h2>
  <div class="kv">
    <div class="item"><div class="l">CPU now</div><div class="v">{{.CPUPct}}%</div></div>
    <div class="item"><div class="l">CPU</div><div class="v">{{.CPU}}</div></div>
    <div class="item"><div class="l">Cores</div><div class="v">{{.Cores}}</div></div>
    {{if .Virtualization}}<div class="item"><div class="l">Platform type</div><div class="v">{{.Virtualization}}</div></div>{{end}}
    {{if .Encryption}}<div class="item"><div class="l">Disk encryption</div><div class="v">{{.Encryption}}</div></div>{{end}}
    <div class="item"><div class="l">Uptime</div><div class="v">{{.Uptime}}</div></div>
    <div class="item"><div class="l">Load average</div><div class="v">{{range .LoadAvg}}{{.}} {{else}}&mdash;{{end}}</div></div>
    <div class="item"><div class="l">Packages</div><div class="v">{{.Packages}}</div></div>
    <div class="item"><div class="l">Last report</div><div class="v">{{.CollectedAt}} ({{.LastSeen}})</div></div>
  </div>

  <h2>Performance (recent)</h2>
  <div style="background:var(--panel);border:1px solid var(--line);border-radius:12px;padding:1rem"><canvas id="perf" height="90"></canvas></div>

  <h2>Memory</h2>
  <div class="metric"><span>{{.MemUsedMB}} / {{.MemTotalMB}} MB</span><span>{{.MemPct}}%</span></div>
  <div class="bar"><i class="{{barclass .MemPct}}" style="{{.MemBar}}"></i></div>

  <h2>Disks</h2>
  <table><tr><th>Mount</th><th>Filesystem</th><th>Used / Total</th><th>Usage</th></tr>
  {{range .Disks}}<tr><td>{{.Mount}}</td><td>{{.FS}}</td><td>{{.UsedMB}} / {{.TotalMB}} MB</td>
    <td><div class="bar" style="width:140px;display:inline-block;vertical-align:middle"><i class="{{barclass .Pct}}" style="{{.Bar}}"></i></div> {{.Pct}}%</td></tr>{{end}}
  </table>

  {{if .Network}}<h2>Network</h2>
  <table><tr><th>Interface</th><th>IPv4 address</th><th>MAC</th></tr>
  {{range .Network}}<tr><td>{{.Name}}</td><td>{{.IPv4}}</td><td class="sub">{{.MAC}}</td></tr>{{end}}
  </table>{{end}}

  {{if .Processes}}<h2>Top processes (by CPU)</h2>
  <table><tr><th>PID</th><th>Process</th><th>CPU %</th><th>Mem %</th></tr>
  {{range .Processes}}<tr><td>{{.PID}}</td><td>{{.Name}}</td><td>{{.CPUPct}}%</td><td>{{.MemPct}}%</td></tr>{{end}}
  </table>{{end}}

  <h2>Services ({{.RunningServices}} running of {{len .Services}})</h2>
  <div class="svc-scroll"><table><tr><th>Service</th><th>Status</th></tr>
  {{range .Services}}<tr><td>{{.Name}}</td><td><span class="dot {{if eq .Status "running"}}d-ok{{else}}d-stale{{end}}"></span>{{.Status}}</td></tr>{{end}}
  </table></div>

  {{if .PackageList}}<h2>Installed packages ({{.Packages}})</h2>
  <input id="pkgSearch" placeholder="Search packages…" style="width:100%;max-width:360px;background:var(--panel);border:1px solid var(--line);border-radius:8px;color:var(--ink);padding:.5rem .7rem;font:inherit;margin-bottom:.4rem">
  <div class="svc-scroll"><table id="pkgTable"><tr><th>Name</th><th>Version</th></tr>
  {{range .PackageList}}{{if .Name}}<tr><td>{{.Name}}</td><td class="sub">{{.Version}}</td></tr>{{end}}{{end}}
  </table></div>
  <script>(function(){var s=document.getElementById('pkgSearch'),t=document.getElementById('pkgTable');if(!s||!t)return;
    s.addEventListener('input',function(){var q=s.value.toLowerCase();Array.prototype.slice.call(t.rows,1).forEach(function(r){
      r.style.display=r.cells[0].textContent.toLowerCase().indexOf(q)>=0?'':'none';});});})();</script>{{end}}
</div>
<script src="https://cdn.jsdelivr.net/npm/chart.js@4"></script>
<script>
  const D = {{.ChartData}};
  if (window.Chart && document.getElementById('perf') && D.labels.length) {
    new Chart(document.getElementById('perf'), {
      type: 'line',
      data: { labels: D.labels, datasets: [
        { label: 'CPU %', data: D.cpu, borderColor: '#6366f1', backgroundColor: 'rgba(99,102,241,.15)', fill: true, tension: .3, pointRadius: 0, borderWidth: 2 },
        { label: 'Memory %', data: D.mem, borderColor: '#22d3ee', backgroundColor: 'rgba(34,211,238,.10)', fill: true, tension: .3, pointRadius: 0, borderWidth: 2 },
        { label: 'Disk %', data: D.disk, borderColor: '#f59e0b', backgroundColor: 'rgba(245,158,11,.08)', fill: true, tension: .3, pointRadius: 0, borderWidth: 2 }
      ]},
      options: { responsive: true, interaction: { intersect: false, mode: 'index' },
        scales: { y: { min: 0, max: 100, ticks: { color: '#94a3b8' }, grid: { color: '#1f2a44' } },
                  x: { ticks: { color: '#94a3b8', maxTicksLimit: 8 }, grid: { color: '#1f2a44' } } },
        plugins: { legend: { labels: { color: '#cbd5e1' } } } }
    });
  }
</script>
`

// shellTop renders the left side panel; every page is wrapped in it. Page data
// must expose Active, UserEmail and CanManageUsers.
const shellTop = `<div class="app">
  {{if .Chrome}}<aside class="side">
    <a href="/" class="brand">Look<b>out</b></a>
    <nav class="sidenav">
      <a href="/" class="{{if eq .Active "overview"}}on{{end}}">&#9638; Overview</a>
      <a href="/guides" class="{{if eq .Active "guides"}}on{{end}}">&#10067; Help &amp; Guides</a>
      <a href="/integrations" class="{{if eq .Active "integrations"}}on{{end}}">&#129513; Integrations</a>
      <a href="/notifications" class="{{if eq .Active "notifications"}}on{{end}}">&#128276; Notifications</a>
      {{if .CanManageUsers}}<a href="/admin/users" class="{{if eq .Active "users"}}on{{end}}">&#128101; Users</a>{{end}}
      <a href="/settings" class="{{if eq .Active "settings"}}on{{end}}">&#9881; Settings</a>
    </nav>
    <div class="side-foot">
      <button class="linkbtn" onclick="lkTheme()" title="Toggle light/dark">&#9681; Theme</button>
      {{if .UserEmail}}<a href="/account">{{.UserEmail}}</a><form method="post" action="/logout" style="margin:0"><button class="linkbtn">Sign out</button></form>{{end}}
    </div>
  </aside>{{end}}
  <main class="content">`

const shellBottom = `</main></div></body></html>`

func mustPage(name, title, content string) *template.Template {
	return template.Must(template.New(name).Funcs(funcs).Parse(
		headOpen + `<title>` + title + ` &middot; Lookout</title>` + styleTag + shellTop + content + shellBottom))
}

var dashboardTmpl = mustPage("dash", "Dashboard", dashBody)
var detailTmpl = mustPage("detail", "{{.ID}}", detailBody)
