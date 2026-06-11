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
`

const headOpen = `<!DOCTYPE html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">`
const styleTag = `<style>` + cssConst + `</style></head><body>`

const dashBody = `
<header class="top"><div class="logo">Look<b>out</b></div>
  {{if .UserEmail}}<div class="topnav">{{if .CanManageUsers}}<a href="/admin/users">Users</a>{{end}}<a href="/account">{{.UserEmail}}</a><form method="post" action="/logout" style="display:inline;margin:0"><button class="linkbtn">Sign out</button></form></div>{{else}}<div class="sub">Control plane</div>{{end}}
</header>
<div class="wrap">
  <div class="summary">
    <span class="chip"><b>{{.Total}}</b> servers</span>
    <span class="chip"><span class="dot d-ok"></span><b>{{.OK}}</b> ok</span>
    <span class="chip"><span class="dot d-warning"></span><b>{{.Warning}}</b> warning</span>
    <span class="chip"><span class="dot d-critical"></span><b>{{.Critical}}</b> critical</span>
    <span class="chip"><span class="dot d-stale"></span><b>{{.Stale}}</b> stale</span>
  </div>
  {{if .Servers}}
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
<footer>Lookout &middot; open-source infrastructure monitoring</footer>
</body></html>`

const detailBody = `
<header class="top"><div class="logo">Look<b>out</b></div>
  {{if .UserEmail}}<div class="topnav">{{if .CanManageUsers}}<a href="/admin/users">Users</a>{{end}}<a href="/account">{{.UserEmail}}</a><form method="post" action="/logout" style="display:inline;margin:0"><button class="linkbtn">Sign out</button></form></div>{{else}}<div class="sub">Control plane</div>{{end}}
</header>
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

  <h2>Services ({{.RunningServices}} running of {{len .Services}})</h2>
  <div class="svc-scroll"><table><tr><th>Service</th><th>Status</th></tr>
  {{range .Services}}<tr><td>{{.Name}}</td><td><span class="dot {{if eq .Status "running"}}d-ok{{else}}d-stale{{end}}"></span>{{.Status}}</td></tr>{{end}}
  </table></div>
</div>
<footer>Lookout &middot; open-source infrastructure monitoring</footer>
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
</body></html>`

var dashboardTmpl = template.Must(template.New("dash").Funcs(funcs).Parse(
	headOpen + `<title>Dashboard &middot; Lookout</title>` + styleTag + dashBody))

var detailTmpl = template.Must(template.New("detail").Funcs(funcs).Parse(
	headOpen + `<title>{{.ID}} &middot; Lookout</title>` + styleTag + detailBody))
