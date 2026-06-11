// Package auth provides user accounts, password + SSO login, TOTP MFA, sessions,
// and role-based access control for the Lookout control plane.
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Role is a coarse permission bundle assigned to a user.
type Role string

const (
	RoleOwner    Role = "owner"
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"
	RoleViewer   Role = "viewer"
)

// Permission is a single capability checked by middleware.
type Permission string

const (
	PermViewDashboard Permission = "view_dashboard"
	PermManageUsers   Permission = "manage_users"
	PermManageAlerts  Permission = "manage_alerts"
	PermManageAgents  Permission = "manage_agents"
)

// rolePerms maps each role to the permissions it grants. This is the granular
// RBAC table; new roles/permissions are added here.
var rolePerms = map[Role][]Permission{
	RoleOwner:    {PermViewDashboard, PermManageUsers, PermManageAlerts, PermManageAgents},
	RoleAdmin:    {PermViewDashboard, PermManageUsers, PermManageAlerts, PermManageAgents},
	RoleOperator: {PermViewDashboard, PermManageAlerts, PermManageAgents},
	RoleViewer:   {PermViewDashboard},
}

// ValidRole reports whether r is a known role.
func ValidRole(r Role) bool { _, ok := rolePerms[r]; return ok }

// Can reports whether a role grants a permission.
func (r Role) Can(p Permission) bool {
	for _, have := range rolePerms[r] {
		if have == p {
			return true
		}
	}
	return false
}

// User is one account.
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name,omitempty"`
	Role         Role      `json:"role"`
	PasswordHash string    `json:"password_hash,omitempty"` // bcrypt; empty for SSO-only
	TOTPSecret   string    `json:"totp_secret,omitempty"`   // base32; empty until MFA set up
	MFAEnabled   bool      `json:"mfa_enabled"`
	Provider     string    `json:"provider,omitempty"` // "", google, github
	Disabled     bool      `json:"disabled,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// Store is a concurrency-safe, file-backed set of users + sessions.
type Store struct {
	mu       sync.RWMutex
	path     string
	users    map[string]*User    // by ID
	sessions map[string]*Session // by token
}

type persisted struct {
	Users    []*User    `json:"users"`
	Sessions []*Session `json:"sessions"`
}

// Open loads the auth store from path, or starts empty.
func Open(path string) (*Store, error) {
	s := &Store{path: path, users: map[string]*User{}, sessions: map[string]*Session{}}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	var p persisted
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	for _, u := range p.Users {
		s.users[u.ID] = u
	}
	for _, sess := range p.Sessions {
		s.sessions[sess.Token] = sess
	}
	return s, nil
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// CreateUser adds a user. Password may be empty for SSO-only accounts.
func (s *Store) CreateUser(email, name string, role Role, password string) (*User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, fmt.Errorf("email is required")
	}
	if !ValidRole(role) {
		return nil, fmt.Errorf("invalid role %q", role)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.users {
		if u.Email == email {
			return nil, fmt.Errorf("a user with that email already exists")
		}
	}
	u := &User{ID: newID(), Email: email, Name: name, Role: role, CreatedAt: time.Now().UTC()}
	if password != "" {
		h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		u.PasswordHash = string(h)
	}
	s.users[u.ID] = u
	return u, s.persist()
}

// Authenticate verifies an email + password and returns the user.
func (s *Store) Authenticate(email, password string) (*User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	s.mu.RLock()
	var u *User
	for _, cand := range s.users {
		if cand.Email == email {
			u = cand
			break
		}
	}
	s.mu.RUnlock()
	if u == nil || u.Disabled || u.PasswordHash == "" {
		// Still run a bcrypt compare against a dummy hash to keep timing uniform.
		_ = bcrypt.CompareHashAndPassword([]byte("$2a$10$invalidinvalidinvalidinvalidinvalidinvalidinvalidinv"), []byte(password))
		return nil, fmt.Errorf("invalid email or password")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, fmt.Errorf("invalid email or password")
	}
	return u, nil
}

// UserByEmail / UserByID look users up.
func (s *Store) UserByEmail(email string) (*User, bool) {
	email = strings.ToLower(strings.TrimSpace(email))
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, u := range s.users {
		if u.Email == email {
			return u, true
		}
	}
	return nil, false
}

func (s *Store) UserByID(id string) (*User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[id]
	return u, ok
}

// ListUsers returns all users sorted by email.
func (s *Store) ListUsers() []*User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*User, 0, len(s.users))
	for _, u := range s.users {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Email < out[j].Email })
	return out
}

// Count returns the number of users (used for first-run bootstrap).
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.users)
}

// update applies fn to the user under lock and persists.
func (s *Store) update(id string, fn func(*User)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return fmt.Errorf("user not found")
	}
	fn(u)
	return s.persist()
}

// persist writes the store atomically. Caller holds the lock.
func (s *Store) persist() error {
	p := persisted{}
	for _, u := range s.users {
		p.Users = append(p.Users, u)
	}
	for _, sess := range s.sessions {
		p.Sessions = append(p.Sessions, sess)
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
