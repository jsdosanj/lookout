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

// Session is a logged-in browser session. MFADone is false for the brief window
// between password verification and TOTP entry.
type Session struct {
	Token   string    `json:"token"`
	UserID  string    `json:"user_id"`
	Expires time.Time `json:"expires"`
	MFADone bool      `json:"mfa_done"`
}

func randomToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// CreateSession starts a session. mfaDone is false when MFA is still pending.
func (s *Store) CreateSession(userID string, mfaDone bool) (*Session, error) {
	sess := &Session{Token: randomToken(), UserID: userID, Expires: time.Now().Add(sessionTTL).UTC(), MFADone: mfaDone}
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

// MarkSessionMFADone promotes a session to fully authenticated.
func (s *Store) MarkSessionMFADone(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[token]
	if !ok {
		return fmt.Errorf("session not found")
	}
	sess.MFADone = true
	return s.persist()
}

// DeleteSession logs out.
func (s *Store) DeleteSession(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
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

// SetRole changes a user's role.
func (s *Store) SetRole(userID string, role Role) error {
	if !ValidRole(role) {
		return fmt.Errorf("invalid role")
	}
	return s.update(userID, func(u *User) { u.Role = role })
}

// SetDisabled enables/disables a user.
func (s *Store) SetDisabled(userID string, disabled bool) error {
	return s.update(userID, func(u *User) { u.Disabled = disabled })
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
