package auth

import (
	"context"
	"net/http"
)

// Auth wires the user store and config into HTTP middleware + handlers.
type Auth struct {
	store        *Store
	secure       bool // emit Secure cookies (set true behind TLS)
	issuer       string
	oauth        map[string]*oauthProvider
	loginLimiter *throttle // password-login brute-force guard
	mfaLimiter   *throttle // TOTP-verify brute-force guard
}

// New creates the auth layer. issuer labels TOTP entries in authenticator apps.
func New(store *Store, secure bool, issuer string) *Auth {
	return &Auth{
		store: store, secure: secure, issuer: issuer, oauth: loadOAuthProviders(),
		loginLimiter: newThrottle(), mfaLimiter: newThrottle(),
	}
}

// Store exposes the user store (for first-run bootstrap, etc.).
func (a *Auth) Store() *Store { return a.store }

type ctxKey int

const (
	userKey ctxKey = iota
	csrfKey
)

// CurrentUser returns the authenticated user attached by RequireAuth, or nil.
func CurrentUser(r *http.Request) *User {
	u, _ := r.Context().Value(userKey).(*User)
	return u
}

// CSRFToken returns the current session's synchronizer token, attached by
// RequireAuth. Handlers in other packages embed it in their POST forms (e.g. the
// dashboard's sign-out form) so the form passes csrf verification.
func CSRFToken(r *http.Request) string {
	t, _ := r.Context().Value(csrfKey).(string)
	return t
}

// load resolves the session + user from the request cookie.
func (a *Auth) load(r *http.Request) (*User, *Session) {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return nil, nil
	}
	sess, ok := a.store.SessionByToken(c.Value)
	if !ok {
		return nil, nil
	}
	u, ok := a.store.UserByID(sess.UserID)
	if !ok || u.Disabled {
		return nil, nil
	}
	return u, sess
}

// RequireAuth allows the request only for a fully authenticated user, else
// redirects to the login (or MFA) page.
func (a *Auth) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, sess := a.load(r)
		if u == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if u.MFAEnabled && !sess.MFADone {
			http.Redirect(w, r, "/login/mfa", http.StatusSeeOther)
			return
		}
		ctx := context.WithValue(r.Context(), userKey, u)
		ctx = context.WithValue(ctx, csrfKey, sess.CSRF)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ProtectPost guards a state-changing POST handler with both a permission check
// and CSRF synchronizer-token verification. It is the exported form other
// packages use to register their own authenticated POST endpoints (the dashboard
// alert-rule and acknowledge actions) with the same protections as the auth
// package's own forms.
func (a *Auth) ProtectPost(p Permission, h http.HandlerFunc) http.Handler {
	return a.RequirePermission(p, a.csrf(h))
}

// RequirePermission allows the request only if the user has the permission.
func (a *Auth) RequirePermission(p Permission, next http.Handler) http.Handler {
	return a.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u := CurrentUser(r); u == nil || !u.Role.Can(p) {
			http.Error(w, "forbidden — you don't have permission for this", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	}))
}
