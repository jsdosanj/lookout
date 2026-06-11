package auth

import "html/template"

const authCSS = `
body{font-family:ui-sans-serif,system-ui,-apple-system,sans-serif;background:#0b1220;color:#e6edf7;margin:0;line-height:1.5}
.wrap{max-width:420px;margin:6vh auto;padding:0 1.5rem}
.wrap.wide{max-width:920px}
.card{background:#121a2e;border:1px solid #1f2a44;border-radius:14px;padding:1.6rem}
.logo{font-weight:800;font-size:1.4rem;text-align:center;margin-bottom:1.2rem}
.logo b{color:#6366f1}
h3{margin:.2rem 0 .4rem}
label{display:block;font-size:.82rem;color:#94a3b8;margin:.8rem 0 .3rem}
input,select{width:100%;box-sizing:border-box;background:#0b1220;border:1px solid #1f2a44;border-radius:8px;color:#e6edf7;padding:.6rem .7rem;font:inherit}
.btn{display:block;width:100%;box-sizing:border-box;background:#6366f1;color:#fff;border:none;border-radius:8px;padding:.65rem;font-weight:700;margin-top:1rem;cursor:pointer;text-align:center;text-decoration:none}
.btn.ghost{background:transparent;border:1px solid #1f2a44;color:#e6edf7;margin-top:.6rem}
.err{background:rgba(239,68,68,.12);color:#fca5a5;border:1px solid rgba(239,68,68,.3);border-radius:8px;padding:.6rem;font-size:.88rem;margin-bottom:1rem}
.muted{color:#94a3b8;font-size:.9rem}
a{color:#818cf8;text-decoration:none}a:hover{text-decoration:underline}
table{width:100%;border-collapse:collapse;font-size:.9rem;margin-top:.4rem}
th,td{text-align:left;padding:.5rem;border-bottom:1px solid #1f2a44;vertical-align:middle}
.bar{display:flex;justify-content:space-between;align-items:center;margin-bottom:1.2rem}
.row{display:flex;gap:.7rem;align-items:center}
code{background:#0b1220;border:1px solid #1f2a44;border-radius:6px;padding:.15rem .4rem;word-break:break-all;font-size:.82rem}
form.inline{display:inline;margin:0}
`

const authHead = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Lookout</title><style>` + authCSS + `summary{cursor:pointer;color:#818cf8}select[multiple]{height:auto}</style></head><body>`
const authFoot = `</body></html>`

var authFuncs = template.FuncMap{
	"has": func(ss []string, x string) bool {
		for _, s := range ss {
			if s == x {
				return true
			}
		}
		return false
	},
}

func authTmpl(body string) *template.Template {
	return template.Must(template.New("t").Funcs(authFuncs).Parse(authHead + body + authFoot))
}

// adminNav is the shared top nav for the admin pages.
const adminNav = `<div class="bar"><div class="logo" style="margin:0">Look<b>out</b> &middot; Admin</div>
  <div class="row" style="gap:1.1rem;font-size:.9rem;flex-wrap:wrap"><a href="/admin/users">Users</a><a href="/admin/org/group">Groups</a><a href="/admin/org/department">Departments</a><a href="/admin/org/location">Locations</a><a href="/">&larr; Dashboard</a></div></div>`

var loginTmpl = authTmpl(`
<div class="wrap"><div class="card">
  <div class="logo">Look<b>out</b></div>
  {{if .Err}}<div class="err">{{if eq .Err "noaccount"}}No Lookout account for that identity — ask an admin to add you.{{else if eq .Err "state"}}Login session expired, please try again.{{else if eq .Err "sso"}}SSO sign-in failed. Try again.{{else}}Sign-in failed. Check your email and password.{{end}}</div>{{end}}
  <form method="post" action="/login">
    <label>Email</label><input name="email" type="email" autocomplete="email" required>
    <label>Password</label><input name="password" type="password" autocomplete="current-password" required>
    <button class="btn" type="submit">Sign in</button>
  </form>
  {{if .Providers}}<p class="muted" style="text-align:center;margin:1.3rem 0 .2rem">or continue with</p>
  {{range .Providers}}<a class="btn ghost" href="/auth/{{.}}/login" style="text-transform:capitalize">{{.}}</a>{{end}}{{end}}
</div></div>`)

var mfaTmpl = authTmpl(`
<div class="wrap"><div class="card">
  <div class="logo">Look<b>out</b></div>
  <p class="muted">Enter the 6-digit code from your authenticator app.</p>
  {{if .Err}}<div class="err">That code didn't match. Try again.</div>{{end}}
  <form method="post" action="/login/mfa">
    <label>Authentication code</label><input name="code" inputmode="numeric" pattern="[0-9]*" autocomplete="one-time-code" required autofocus>
    <button class="btn" type="submit">Verify</button>
  </form>
</div></div>`)

var accountTmpl = authTmpl(`
<div class="wrap"><div class="card">
  <div class="bar"><div class="logo" style="margin:0">Look<b>out</b></div><a href="/">&larr; Dashboard</a></div>
  <h3>Your account</h3>
  <p class="muted">{{.User.Email}} &middot; role: {{.User.Role}}</p>
  <h3 style="margin-top:1.4rem">Two-factor authentication</h3>
  {{if .User.MFAEnabled}}
    <p class="muted">MFA is <b style="color:#22c55e">on</b>.</p>
    <form method="post" action="/account/mfa/disable"><button class="btn ghost" type="submit">Turn off MFA</button></form>
  {{else}}
    <p class="muted">Add a second factor with an authenticator app (Google Authenticator, 1Password, …).</p>
    <form method="post" action="/account/mfa/begin"><button class="btn" type="submit">Set up MFA</button></form>
  {{end}}
  <form method="post" action="/logout"><button class="btn ghost" type="submit">Sign out</button></form>
</div></div>`)

var mfaSetupTmpl = authTmpl(`
<div class="wrap"><div class="card">
  <div class="logo">Look<b>out</b></div>
  <h3>Set up two-factor</h3>
  <p class="muted">Add this secret to your authenticator app, then enter the current code to confirm.</p>
  {{if .Err}}<div class="err">{{.Err}}</div>{{end}}
  <label>Secret key</label><p><code>{{.Secret}}</code></p>
  <label>Setup link (otpauth)</label><p><code>{{.URI}}</code></p>
  <form method="post" action="/account/mfa/enable">
    <label>Current 6-digit code</label><input name="code" inputmode="numeric" pattern="[0-9]*" required autofocus>
    <button class="btn" type="submit">Enable MFA</button>
  </form>
  <a class="btn ghost" href="/account">Cancel</a>
</div></div>`)

var usersTmpl = authTmpl(`
<div class="wrap wide"><div class="card">
  ` + adminNav + `
  {{if .Err}}<div class="err">{{.Err}}</div>{{end}}
  <h3>Users</h3>
  <table>
    <tr><th>Email</th><th>Name</th><th>Role</th><th>MFA</th><th>Status</th><th>Organization</th><th></th></tr>
    {{range .Users}}
    <tr>
      <td>{{.Email}}{{if .Provider}} <span class="muted">({{.Provider}})</span>{{end}}</td>
      <td>{{.Name}}</td>
      <td>
        <form class="inline" method="post" action="/admin/users/role">
          <input type="hidden" name="id" value="{{.ID}}">
          {{$r := .Role}}<select name="role" onchange="this.form.submit()">{{range $.Roles}}<option value="{{.}}" {{if eq . $r}}selected{{end}}>{{.}}</option>{{end}}</select>
        </form>
      </td>
      <td>{{if .MFAEnabled}}<span style="color:#22c55e">on</span>{{else}}off{{end}}</td>
      <td>{{if .Disabled}}<span style="color:#f59e0b">disabled</span>{{else}}active{{end}}</td>
      <td>
        <details><summary>Edit</summary>
          <form method="post" action="/admin/users/org" style="margin-top:.5rem;min-width:220px">
            <input type="hidden" name="id" value="{{.ID}}">
            <label>Department</label>{{$d := .DepartmentID}}<select name="department"><option value="">—</option>{{range $.Departments}}<option value="{{.ID}}" {{if eq .ID $d}}selected{{end}}>{{.Name}}</option>{{end}}</select>
            <label>Location</label>{{$l := .LocationID}}<select name="location"><option value="">—</option>{{range $.Locations}}<option value="{{.ID}}" {{if eq .ID $l}}selected{{end}}>{{.Name}}</option>{{end}}</select>
            <label>Groups</label>{{$g := .GroupIDs}}<select name="groups" multiple size="3">{{range $.Groups}}<option value="{{.ID}}" {{if has $g .ID}}selected{{end}}>{{.Name}}</option>{{end}}</select>
            <button class="btn" type="submit">Save</button>
          </form>
        </details>
      </td>
      <td>
        {{if ne .ID $.Me.ID}}
        <form class="inline" method="post" action="/admin/users/disable">
          <input type="hidden" name="id" value="{{.ID}}">
          <input type="hidden" name="disabled" value="{{if .Disabled}}false{{else}}true{{end}}">
          <button class="btn ghost" style="width:auto;margin:0;padding:.3rem .6rem">{{if .Disabled}}Enable{{else}}Disable{{end}}</button>
        </form>{{end}}
      </td>
    </tr>
    {{end}}
  </table>
  <h3 style="margin-top:1.6rem">Add a user</h3>
  <form method="post" action="/admin/users/create">
    <div class="row" style="flex-wrap:wrap">
      <div style="flex:1;min-width:170px"><label>Email</label><input name="email" type="email" required></div>
      <div style="flex:1;min-width:140px"><label>Name</label><input name="name"></div>
      <div style="min-width:130px"><label>Role</label><select name="role">{{range .Roles}}<option value="{{.}}">{{.}}</option>{{end}}</select></div>
    </div>
    <label>Temporary password (leave blank for SSO-only)</label><input name="password" type="password">
    <button class="btn" type="submit">Create user</button>
  </form>
</div></div>`)

var orgTmpl = authTmpl(`
<div class="wrap wide"><div class="card">
  ` + adminNav + `
  {{if .Err}}<div class="err">{{.Err}}</div>{{end}}
  <h3>{{.Title}}</h3>
  {{if .Units}}
  <table><tr><th>Name</th><th>{{.DetailLabel}}</th><th></th></tr>
    {{range .Units}}<tr><td><b>{{.Name}}</b></td><td class="muted">{{.Detail}}</td>
      <td><form class="inline" method="post" action="/admin/org/{{.Kind}}/delete"><input type="hidden" name="id" value="{{.ID}}"><button class="btn ghost" style="width:auto;margin:0;padding:.3rem .6rem">Delete</button></form></td></tr>{{end}}
  </table>
  {{else}}<p class="muted">None yet — add one below.</p>{{end}}
  <h3 style="margin-top:1.4rem">Add</h3>
  <form method="post" action="/admin/org/{{.Kind}}/create">
    <div class="row" style="flex-wrap:wrap">
      <div style="flex:1;min-width:160px"><label>Name</label><input name="name" required></div>
      <div style="flex:2;min-width:200px"><label>{{.DetailLabel}}</label><input name="detail"></div>
    </div>
    <button class="btn" type="submit">Create</button>
  </form>
</div></div>`)
