package auth

import (
	"crypto/subtle"
	"html/template"
	"net/http"
	"time"
)

// CSRF protection uses synchronizer tokens. For authenticated requests the token
// is bound to the session (Session.CSRF). For the pre-authentication forms
// (login, MFA entry) there is no full session yet, so a dedicated, short-lived
// CSRF cookie carries the token. Every state-changing POST must echo the token
// in a hidden form field, verified in constant time. SameSite=Lax on the cookies
// remains as defense-in-depth.

// CSRFField is the hidden form input name carrying the synchronizer token.
const CSRFField = "csrf_token"

const csrfCookie = "lookout_csrf"

// expectedCSRF returns the token the next POST must echo: the session token when
// authenticated, else the pre-auth cookie token (an empty string if neither
// exists yet — callers issue one with ensureCSRFCookie when rendering forms).
func (a *Auth) expectedCSRF(r *http.Request) string {
	if _, sess := a.load(r); sess != nil && sess.CSRF != "" {
		return sess.CSRF
	}
	if c, err := r.Cookie(csrfCookie); err == nil {
		return c.Value
	}
	return ""
}

// ensureCSRFCookie returns a CSRF token for an anonymous form page, minting and
// setting the cookie if one isn't already present. Used by pre-auth GET pages
// (login, MFA) so their POST can be verified.
func (a *Auth) ensureCSRFCookie(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(csrfCookie); err == nil && c.Value != "" {
		return c.Value
	}
	tok := randomToken()
	http.SetCookie(w, &http.Cookie{
		Name: csrfCookie, Value: tok, Path: "/",
		HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode,
		Expires: time.Now().Add(preMFASessionTTL),
	})
	return tok
}

// csrfTokenFor resolves the token to embed in a form on a page render: the
// session token if authenticated, otherwise a freshly issued pre-auth cookie token.
func (a *Auth) csrfTokenFor(w http.ResponseWriter, r *http.Request) string {
	if _, sess := a.load(r); sess != nil && sess.CSRF != "" {
		return sess.CSRF
	}
	return a.ensureCSRFCookie(w, r)
}

// checkCSRF verifies the submitted synchronizer token against the expected one
// in constant time. It reports false on any mismatch or when no token is known.
func (a *Auth) checkCSRF(r *http.Request) bool {
	want := a.expectedCSRF(r)
	if want == "" {
		return false
	}
	got := r.FormValue(CSRFField)
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// csrfField renders the hidden CSRF input for embedding in a template via the
// "csrf" func; tok is the per-request token from csrfTokenFor.
func csrfField(tok string) template.HTML {
	return template.HTML(`<input type="hidden" name="` + CSRFField + `" value="` + template.HTMLEscapeString(tok) + `">`)
}
