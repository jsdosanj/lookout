package store

import (
	"path/filepath"
	"testing"
)

func TestAgentEnrollAndTokenLookup(t *testing.T) {
	as, err := OpenAgents(filepath.Join(t.TempDir(), "agents.json"))
	if err != nil {
		t.Fatal(err)
	}
	id, tok, err := as.Enroll("web-1")
	if err != nil {
		t.Fatal(err)
	}
	if id == "" || tok == "" {
		t.Fatal("enroll returned empty id/token")
	}
	a, ok := as.AgentByToken(tok)
	if !ok || a.ID != id {
		t.Fatalf("token lookup failed: ok=%v", ok)
	}
	if _, ok := as.AgentByToken("nope"); ok {
		t.Error("bogus token accepted")
	}
	if _, ok := as.AgentByToken(""); ok {
		t.Error("empty token accepted")
	}
}

func TestHostnameTOFUPinning(t *testing.T) {
	as, err := OpenAgents(filepath.Join(t.TempDir(), "agents.json"))
	if err != nil {
		t.Fatal(err)
	}
	// First identity pins the hostname.
	if err := as.BindHostname("db-1", "agentA"); err != nil {
		t.Fatalf("first bind should succeed: %v", err)
	}
	// Same identity re-reporting is fine.
	if err := as.BindHostname("db-1", "agentA"); err != nil {
		t.Fatalf("rebind by owner should succeed: %v", err)
	}
	// A different identity claiming the same hostname is rejected.
	if err := as.BindHostname("db-1", "agentB"); err == nil {
		t.Error("cross-identity hostname claim was NOT rejected")
	}
	// Even the shared identity can't steal an already-pinned hostname.
	if err := as.BindHostname("db-1", SharedIdentity); err == nil {
		t.Error("shared identity overwrote a pinned hostname")
	}
}

func TestAgentStorePersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agents.json")
	as, err := OpenAgents(path)
	if err != nil {
		t.Fatal(err)
	}
	id, tok, err := as.Enroll("host-x")
	if err != nil {
		t.Fatal(err)
	}
	if err := as.BindHostname("host-x", id); err != nil {
		t.Fatal(err)
	}
	// Reopen and confirm the token + pin survived.
	as2, err := OpenAgents(path)
	if err != nil {
		t.Fatal(err)
	}
	if a, ok := as2.AgentByToken(tok); !ok || a.ID != id {
		t.Error("agent token not persisted")
	}
	// The pin must still reject a different identity.
	if err := as2.BindHostname("host-x", "intruder"); err == nil {
		t.Error("pin not persisted across reopen")
	}
}
