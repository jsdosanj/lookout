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

const userKey ctxKey = 0

// CurrentUser returns the authenticated user attached by RequireAuth, or nil.
func CurrentUser(r *http.Request) *User {
	u, _ := r.Context().Value(userKey).(*User)
	return u
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
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
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
