package envelope

import (
	"crypto/ed25519"
	"encoding/base64"
	"testing"

	"github.com/jsdosanj/lookout/internal/collect/identity"
	"github.com/jsdosanj/lookout/internal/collector"
)

// enrolledIdentity returns a freshly-generated identity bound to a test agent.
func enrolledIdentity(t *testing.T) *identity.Identity {
	t.Helper()
	id, err := identity.Generate()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if err := id.Apply(identity.EnrollResponse{AgentID: "agent-1", TenantID: "tenant-1"}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	return id
}

// TestBuildAndVerify proves the signature the agent produces verifies against
// the agent's public key using the SAME canonical SigningBytes the ingest
// Worker would reconstruct — the core integrity contract.
func TestBuildAndVerify(t *testing.T) {
	id := enrolledIdentity(t)
	rec := collector.Record{SchemaID: "lookout.inventory.v1", Payload: map[string]any{"hostname": "h1"}}

	payload, err := MarshalPayload(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	env, err := Build(id, "system_inventory", rec, "", payload)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	// Worker side: reconstruct canonical bytes and verify.
	sig, err := base64.StdEncoding.DecodeString(env.Signature)
	if err != nil {
		t.Fatalf("decode sig: %v", err)
	}
	if !ed25519.Verify(ed25519.PublicKey(id.PubKey), SigningBytes(env, payload), sig) {
		t.Fatal("signature did not verify against the agent pubkey")
	}
}

// TestTamperDetected proves any change to a signed field (here AgentID) breaks
// verification — i.e. a spoofed/altered envelope is rejected.
func TestTamperDetected(t *testing.T) {
	id := enrolledIdentity(t)
	rec := collector.Record{SchemaID: "lookout.posture.v1", Payload: map[string]any{"x": 1}}
	payload, _ := MarshalPayload(rec)
	env, _ := Build(id, "posture", rec, "", payload)

	env.AgentID = "attacker-agent" // tamper
	sig, _ := base64.StdEncoding.DecodeString(env.Signature)
	if ed25519.Verify(ed25519.PublicKey(id.PubKey), SigningBytes(env, payload), sig) {
		t.Fatal("tampered envelope must NOT verify")
	}
}

// TestPayloadBinding proves the signature binds the payload bytes: swapping the
// payload after signing breaks verification even if envelope fields are intact.
func TestPayloadBinding(t *testing.T) {
	id := enrolledIdentity(t)
	rec := collector.Record{SchemaID: "lookout.software.v1", Payload: map[string]any{"count": 1}}
	payload, _ := MarshalPayload(rec)
	env, _ := Build(id, "software", rec, "", payload)

	sig, _ := base64.StdEncoding.DecodeString(env.Signature)
	tampered := []byte(`{"count":9999}`)
	if ed25519.Verify(ed25519.PublicKey(id.PubKey), SigningBytes(env, tampered), sig) {
		t.Fatal("payload swap must break verification")
	}
}

// TestNonceUnique proves nonces don't repeat (anti-replay precondition).
func TestNonceUnique(t *testing.T) {
	id := enrolledIdentity(t)
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		n := id.NextNonce()
		if seen[n] {
			t.Fatalf("duplicate nonce at i=%d", i)
		}
		seen[n] = true
	}
}
