package auth

import (
	"html/template"
	"net/http"
	"net/url"
	"sort"
)

// Mount registers all auth routes (login, MFA, account, admin users, OAuth) on mux.
func (a *Auth) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /login", a.loginPage)
	mux.HandleFunc("POST /login", a.loginPost)
	mux.HandleFunc("GET /login/mfa", a.mfaPage)
	mux.HandleFunc("POST /login/mfa", a.mfaPost)
	mux.HandleFunc("POST /logout", a.logout)

	mux.Handle("GET /account", a.RequireAuth(http.HandlerFunc(a.accountPage)))
	mux.Handle("POST /account/mfa/begin", a.RequireAuth(http.HandlerFunc(a.mfaBegin)))
	mux.Handle("POST /account/mfa/enable", a.RequireAuth(http.HandlerFunc(a.mfaEnable)))
	mux.Handle("POST /account/mfa/disable", a.RequireAuth(http.HandlerFunc(a.mfaDisable)))

	admin := func(h http.HandlerFunc) http.Handler { return a.RequirePermission(PermManageUsers, h) }
	mux.Handle("GET /admin/users", admin(a.usersPage))
	mux.Handle("POST /admin/users/create", admin(a.userCreate))
	mux.Handle("POST /admin/users/role", admin(a.userRole))
	mux.Handle("POST /admin/users/disable", admin(a.userDisable))
	mux.Handle("POST /admin/users/org", admin(a.userOrg))
	mux.Handle("GET /admin/org/{kind}", admin(a.orgPage))
	mux.Handle("POST /admin/org/{kind}/create", admin(a.orgCreate))
	mux.Handle("POST /admin/org/{kind}/delete", admin(a.orgDelete))

	mux.HandleFunc("GET /auth/{provider}/login", a.oauthLogin)
	mux.HandleFunc("GET /auth/{provider}/callback", a.oauthCallback)
}

// ── login / logout ───────────────────────────────────────────────────────────

func (a *Auth) loginPage(w http.ResponseWriter, r *http.Request) {
	if u, _ := a.load(r); u != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	render(w, loginTmpl, map[string]any{
		"Err":       r.URL.Query().Get("err"),
		"Providers": a.providerNames(),
	})
}

func (a *Auth) loginPost(w http.ResponseWriter, r *http.Request) {
	u, err := a.store.Authenticate(r.FormValue("email"), r.FormValue("password"))
	if err != nil {
		http.Redirect(w, r, "/login?err=bad", http.StatusSeeOther)
		return
	}
	mfaDone := !u.MFAEnabled
	sess, err := a.store.CreateSession(u.ID, mfaDone)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	setSessionCookie(w, sess.Token, a.secure)
	if mfaDone {
		http.Redirect(w, r, "/", http.StatusSeeOther)
	} else {
		http.Redirect(w, r, "/login/mfa", http.StatusSeeOther)
	}
}

func (a *Auth) mfaPage(w http.ResponseWriter, r *http.Request) {
	u, sess := a.load(r)
	if u == nil || sess.MFADone {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	render(w, mfaTmpl, map[string]any{"Err": r.URL.Query().Get("err")})
}

func (a *Auth) mfaPost(w http.ResponseWriter, r *http.Request) {
	u, sess := a.load(r)
	if u == nil || sess.MFADone {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if !ValidateTOTP(u.TOTPSecret, r.FormValue("code")) {
		http.Redirect(w, r, "/login/mfa?err=bad", http.StatusSeeOther)
		return
	}
	_ = a.store.MarkSessionMFADone(sess.Token)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *Auth) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(CookieName); err == nil {
		_ = a.store.DeleteSession(c.Value)
	}
	clearSessionCookie(w, a.secure)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// ── account (self-service MFA) ───────────────────────────────────────────────

func (a *Auth) accountPage(w http.ResponseWriter, r *http.Request) {
	u := CurrentUser(r)
	render(w, accountTmpl, map[string]any{"User": u})
}

func (a *Auth) mfaBegin(w http.ResponseWriter, r *http.Request) {
	u := CurrentUser(r)
	secret, err := a.store.BeginMFA(u.ID)
	if err != nil {
		http.Error(w, "could not start MFA", http.StatusInternalServerError)
		return
	}
	render(w, mfaSetupTmpl, map[string]any{
		"Secret": secret,
		"URI":    totpURI(a.issuer, u.Email, secret),
	})
}

func (a *Auth) mfaEnable(w http.ResponseWriter, r *http.Request) {
	u := CurrentUser(r)
	if !ValidateTOTP(u.TOTPSecret, r.FormValue("code")) {
		render(w, mfaSetupTmpl, map[string]any{
			"Secret": u.TOTPSecret, "URI": totpURI(a.issuer, u.Email, u.TOTPSecret), "Err": "That code didn't match — try again.",
		})
		return
	}
	_ = a.store.EnableMFA(u.ID)
	http.Redirect(w, r, "/account", http.StatusSeeOther)
}

func (a *Auth) mfaDisable(w http.ResponseWriter, r *http.Request) {
	_ = a.store.DisableMFA(CurrentUser(r).ID)
	http.Redirect(w, r, "/account", http.StatusSeeOther)
}

// ── admin: users ─────────────────────────────────────────────────────────────

func (a *Auth) usersPage(w http.ResponseWriter, r *http.Request) {
	render(w, usersTmpl, map[string]any{
		"Nav":         template.HTML(adminNav),
		"Me":          CurrentUser(r),
		"Users":       a.store.ListUsers(),
		"Roles":       []Role{RoleOwner, RoleAdmin, RoleOperator, RoleViewer},
		"Departments": a.store.ListOrgUnits("department"),
		"Locations":   a.store.ListOrgUnits("location"),
		"Groups":      a.store.ListOrgUnits("group"),
		"Err":         r.URL.Query().Get("err"),
	})
}

func (a *Auth) userOrg(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	_ = a.store.SetUserOrg(r.FormValue("id"), r.FormValue("department"), r.FormValue("location"), r.Form["groups"])
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

// ── admin: groups / departments / locations ─────────────────────────────────

func kindTitle(kind string) string {
	switch kind {
	case "group":
		return "Groups"
	case "department":
		return "Departments"
	case "location":
		return "Locations & buildings"
	}
	return kind
}

func kindDetailLabel(kind string) string {
	if kind == "location" {
		return "Address"
	}
	return "Description"
}

func (a *Auth) orgPage(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if !ValidKind(kind) {
		http.NotFound(w, r)
		return
	}
	render(w, orgTmpl, map[string]any{
		"Nav":         template.HTML(adminNav),
		"Kind":        kind,
		"Title":       kindTitle(kind),
		"DetailLabel": kindDetailLabel(kind),
		"Units":       a.store.ListOrgUnits(kind),
		"Err":         r.URL.Query().Get("err"),
	})
}

func (a *Auth) orgCreate(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if _, err := a.store.CreateOrgUnit(kind, r.FormValue("name"), r.FormValue("detail")); err != nil {
		http.Redirect(w, r, "/admin/org/"+kind+"?err="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/admin/org/"+kind, http.StatusSeeOther)
}

func (a *Auth) orgDelete(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	_ = a.store.DeleteOrgUnit(r.FormValue("id"))
	http.Redirect(w, r, "/admin/org/"+kind, http.StatusSeeOther)
}

func (a *Auth) userCreate(w http.ResponseWriter, r *http.Request) {
	_, err := a.store.CreateUser(r.FormValue("email"), r.FormValue("name"), Role(r.FormValue("role")), r.FormValue("password"))
	if err != nil {
		http.Redirect(w, r, "/admin/users?err="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

func (a *Auth) userRole(w http.ResponseWriter, r *http.Request) {
	_ = a.store.SetRole(r.FormValue("id"), Role(r.FormValue("role")))
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

func (a *Auth) userDisable(w http.ResponseWriter, r *http.Request) {
	_ = a.store.SetDisabled(r.FormValue("id"), r.FormValue("disabled") == "true")
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

// ── OAuth / SSO ──────────────────────────────────────────────────────────────

const oauthStateCookie = "lookout_oauth_state"

func (a *Auth) oauthLogin(w http.ResponseWriter, r *http.Request) {
	p, ok := a.oauth[r.PathValue("provider")]
	if !ok {
		http.Error(w, "SSO provider not configured", http.StatusNotFound)
		return
	}
	state := randomToken()
	http.SetCookie(w, &http.Cookie{Name: oauthStateCookie, Value: state, Path: "/", HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode, MaxAge: 600})
	http.Redirect(w, r, p.authCodeURL(state), http.StatusSeeOther)
}

func (a *Auth) oauthCallback(w http.ResponseWriter, r *http.Request) {
	p, ok := a.oauth[r.PathValue("provider")]
	if !ok {
		http.Error(w, "SSO provider not configured", http.StatusNotFound)
		return
	}
	c, err := r.Cookie(oauthStateCookie)
	if err != nil || c.Value == "" || c.Value != r.URL.Query().Get("state") {
		http.Redirect(w, r, "/login?err=state", http.StatusSeeOther)
		return
	}
	tok, err := p.exchange(r.URL.Query().Get("code"))
	if err != nil {
		http.Redirect(w, r, "/login?err=sso", http.StatusSeeOther)
		return
	}
	email, _, err := p.email(tok)
	if err != nil {
		http.Redirect(w, r, "/login?err=sso", http.StatusSeeOther)
		return
	}
	// SSO only logs in accounts an admin already created (no silent self-provisioning).
	u, ok := a.store.UserByEmail(email)
	if !ok || u.Disabled {
		http.Redirect(w, r, "/login?err=noaccount", http.StatusSeeOther)
		return
	}
	sess, err := a.store.CreateSession(u.ID, !u.MFAEnabled)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	setSessionCookie(w, sess.Token, a.secure)
	if u.MFAEnabled {
		http.Redirect(w, r, "/login/mfa", http.StatusSeeOther)
	} else {
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func (a *Auth) providerNames() []string {
	var out []string
	for n := range a.oauth {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func render(w http.ResponseWriter, t *template.Template, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = t.Execute(w, data)
}
