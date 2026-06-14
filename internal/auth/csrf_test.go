package auth

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestAuth(t *testing.T) *Auth {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "users.json"))
	if err != nil {
		t.Fatal(err)
	}
	return New(st, false, "Lookout")
}

// post builds a form POST request, optionally carrying a session cookie and a
// CSRF form field.
func formPost(csrfField, sessionToken string) *http.Request {
	body := ""
	if csrfField != "" {
		body = CSRFField + "=" + csrfField
	}
	req := httptest.NewRequest(http.MethodPost, "/logout", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if sessionToken != "" {
		req.AddCookie(&http.Cookie{Name: CookieName, Value: sessionToken})
	}
	return req
}

func TestCSRFRejectsMissingToken(t *testing.T) {
	a := newTestAuth(t)
	u, err := a.store.CreateUser("u@example.com", "U", RoleAdmin, "pw123456")
	if err != nil {
		t.Fatal(err)
	}
	sess, err := a.store.CreateSession(u.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if sess.CSRF == "" {
		t.Fatal("session created without a CSRF token")
	}

	called := false
	h := a.csrf(func(w http.ResponseWriter, r *http.Request) { called = true })

	// Missing CSRF field -> 403, handler not invoked.
	w := httptest.NewRecorder()
	h(w, formPost("", sess.Token))
	if w.Code != http.StatusForbidden {
		t.Errorf("missing token: want 403, got %d", w.Code)
	}
	if called {
		t.Error("handler ran despite missing CSRF token")
	}

	// Wrong CSRF field -> 403.
	w = httptest.NewRecorder()
	h(w, formPost("not-the-token", sess.Token))
	if w.Code != http.StatusForbidden {
		t.Errorf("wrong token: want 403, got %d", w.Code)
	}

	// Correct CSRF field -> handler runs.
	called = false
	w = httptest.NewRecorder()
	h(w, formPost(sess.CSRF, sess.Token))
	if w.Code != http.StatusOK {
		t.Errorf("valid token: want 200, got %d", w.Code)
	}
	if !called {
		t.Error("handler did not run with a valid CSRF token")
	}
}

func TestSessionGCRemovesExpired(t *testing.T) {
	a := newTestAuth(t)
	u, err := a.store.CreateUser("g@example.com", "G", RoleViewer, "pw123456")
	if err != nil {
		t.Fatal(err)
	}
	sess, err := a.store.CreateSession(u.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	// Force-expire the session in place.
	a.store.mu.Lock()
	a.store.sessions[sess.Token].Expires = time.Now().Add(-time.Hour)
	a.store.mu.Unlock()

	if n := a.store.gcExpiredSessions(); n != 1 {
		t.Errorf("gc removed %d sessions, want 1", n)
	}
	a.store.mu.RLock()
	_, present := a.store.sessions[sess.Token]
	a.store.mu.RUnlock()
	if present {
		t.Error("expired session not removed by GC")
	}
}
