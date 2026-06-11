package auth

import (
	"path/filepath"
	"testing"
	"time"
)

func TestTOTPRoundTrip(t *testing.T) {
	secret, err := newTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	code, err := hotp(secret, uint64(time.Now().Unix()/30))
	if err != nil {
		t.Fatal(err)
	}
	if !ValidateTOTP(secret, code) {
		t.Error("valid current code rejected")
	}
	if ValidateTOTP(secret, "000000") && code != "000000" {
		t.Error("obviously wrong code accepted")
	}
	if ValidateTOTP(secret, "12345") { // wrong length
		t.Error("short code accepted")
	}
}

func TestRBAC(t *testing.T) {
	if !RoleViewer.Can(PermViewDashboard) {
		t.Error("viewer should view dashboard")
	}
	if RoleViewer.Can(PermManageUsers) {
		t.Error("viewer should NOT manage users")
	}
	if !RoleOwner.Can(PermManageUsers) || !RoleAdmin.Can(PermManageUsers) {
		t.Error("owner/admin should manage users")
	}
	if RoleOperator.Can(PermManageUsers) {
		t.Error("operator should NOT manage users")
	}
}

func TestUserAuthAndSession(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "users.json"))
	if err != nil {
		t.Fatal(err)
	}
	u, err := st.CreateUser("Admin@Example.com", "Admin", RoleAdmin, "s3cret-pass")
	if err != nil {
		t.Fatal(err)
	}
	if u.Email != "admin@example.com" {
		t.Errorf("email not normalized: %q", u.Email)
	}
	// correct password
	if got, err := st.Authenticate("admin@example.com", "s3cret-pass"); err != nil || got.ID != u.ID {
		t.Errorf("authenticate failed: %v", err)
	}
	// wrong password
	if _, err := st.Authenticate("admin@example.com", "nope"); err == nil {
		t.Error("wrong password accepted")
	}
	// duplicate email
	if _, err := st.CreateUser("admin@example.com", "", RoleViewer, "x"); err == nil {
		t.Error("duplicate email allowed")
	}
	// session lifecycle
	sess, err := st.CreateSession(u.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := st.SessionByToken(sess.Token); !ok || got.UserID != u.ID {
		t.Error("session lookup failed")
	}
	if err := st.DeleteSession(sess.Token); err != nil {
		t.Fatal(err)
	}
	if _, ok := st.SessionByToken(sess.Token); ok {
		t.Error("session not deleted")
	}
	// persistence across reopen
	st2, err := Open(st.path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := st2.UserByEmail("admin@example.com"); !ok {
		t.Error("user not persisted")
	}
}
