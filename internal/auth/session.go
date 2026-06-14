package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// CookieName is the session cookie.
const CookieName = "lookout_session"

const sessionTTL = 12 * time.Hour

// preMFASessionTTL bounds the window between password verification and TOTP
// entry; a half-authenticated session should be short-lived.
const preMFASessionTTL = 10 * time.Minute

// Session is a logged-in browser session. MFADone is false for the brief window
// between password verification and TOTP entry.
type Session struct {
	Token   string    `json:"token"`
	UserID  string    `json:"user_id"`
	Expires time.Time `json:"expires"`
	MFADone bool      `json:"mfa_done"`
	CSRF    string    `json:"csrf"` // per-session synchronizer token for state-changing POSTs
}

func randomToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// CreateSession starts a session. mfaDone is false when MFA is still pending;
// such pre-MFA sessions get a shorter TTL.
func (s *Store) CreateSession(userID string, mfaDone bool) (*Session, error) {
	ttl := sessionTTL
	if !mfaDone {
		ttl = preMFASessionTTL
	}
	sess := &Session{Token: randomToken(), UserID: userID, Expires: time.Now().Add(ttl).UTC(), MFADone: mfaDone, CSRF: randomToken()}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sess.Token] = sess
	return sess, s.persist()
}

// SessionByToken returns a non-expired session.
func (s *Store) SessionByToken(token string) (*Session, bool) {
	s.mu.RLock()
	sess, ok := s.sessions[token]
	s.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Now().After(sess.Expires) {
		_ = s.DeleteSession(token)
		return nil, false
	}
	return sess, true
}

// MarkSessionMFADone promotes a pre-MFA session to fully authenticated by
// minting a brand-new session token and discarding the old one (preventing
// session fixation across the privilege transition). It returns the new
// session; the caller must re-set the cookie with setSessionCookie.
func (s *Store) MarkSessionMFADone(token string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.sessions[token]
	if !ok {
		return nil, fmt.Errorf("session not found")
	}
	delete(s.sessions, token)
	sess := &Session{Token: randomToken(), UserID: old.UserID, Expires: time.Now().Add(sessionTTL).UTC(), MFADone: true, CSRF: randomToken()}
	s.sessions[sess.Token] = sess
	return sess, s.persist()
}

// gcExpiredSessions removes every session past its expiry and persists if any
// were deleted. Returns the number removed.
func (s *Store) gcExpiredSessions() int {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for tok, sess := range s.sessions {
		if now.After(sess.Expires) {
			delete(s.sessions, tok)
			n++
		}
	}
	if n > 0 {
		_ = s.persist()
	}
	return n
}

// StartSessionGC runs a background sweep that deletes expired sessions on each
// tick until stop is closed. It returns immediately; the goroutine exits cleanly
// when stop is closed (no leak). interval <= 0 defaults to 10 minutes.
func (s *Store) StartSessionGC(interval time.Duration, stop <-chan struct{}) {
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				s.gcExpiredSessions()
			}
		}
	}()
}

// DeleteSession logs out.
func (s *Store) DeleteSession(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
	return s.persist()
}

// DeleteUserSessions revokes every session belonging to a user. Called on
// privilege changes (role/disable) so stale sessions can't outlive them.
func (s *Store) DeleteUserSessions(userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for tok, sess := range s.sessions {
		if sess.UserID == userID {
			delete(s.sessions, tok)
		}
	}
	return s.persist()
}

// ── cookie helpers ───────────────────────────────────────────────────────────

func setSessionCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name: CookieName, Value: token, Path: "/",
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
		Expires: time.Now().Add(sessionTTL),
	})
}

func clearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name: CookieName, Value: "", Path: "/",
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
}

// ── user mutations ───────────────────────────────────────────────────────────

// SetPassword updates a user's password.
func (s *Store) SetPassword(userID, password string) error {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.update(userID, func(u *User) { u.PasswordHash = string(h) })
}

// SetRole changes a user's role. All of the user's sessions are revoked so the
// new privileges take effect on a fresh login (no stale elevated sessions).
func (s *Store) SetRole(userID string, role Role) error {
	if !ValidRole(role) {
		return fmt.Errorf("invalid role")
	}
	if err := s.update(userID, func(u *User) { u.Role = role }); err != nil {
		return err
	}
	return s.DeleteUserSessions(userID)
}

// SetDisabled enables/disables a user. Disabling (or re-enabling) revokes all of
// the user's sessions immediately.
func (s *Store) SetDisabled(userID string, disabled bool) error {
	if err := s.update(userID, func(u *User) { u.Disabled = disabled }); err != nil {
		return err
	}
	return s.DeleteUserSessions(userID)
}

// BeginMFA stores a fresh TOTP secret (not yet enabled) and returns it.
func (s *Store) BeginMFA(userID string) (string, error) {
	secret, err := newTOTPSecret()
	if err != nil {
		return "", err
	}
	if err := s.update(userID, func(u *User) { u.TOTPSecret = secret; u.MFAEnabled = false }); err != nil {
		return "", err
	}
	return secret, nil
}

// EnableMFA flips MFA on after the user proves a valid code.
func (s *Store) EnableMFA(userID string) error {
	return s.update(userID, func(u *User) { u.MFAEnabled = true })
}

// DisableMFA turns MFA off and clears the secret.
func (s *Store) DisableMFA(userID string) error {
	return s.update(userID, func(u *User) { u.MFAEnabled = false; u.TOTPSecret = "" })
}
