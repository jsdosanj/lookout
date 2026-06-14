// Package identity is the Lookout agent's cryptographic identity: an Ed25519
// device keypair, enrollment against the Keystone ingest control plane, and
// message signing used by the envelope/transport layers.
//
// Threat model (from plan §10):
//   - Rogue / spoofed agent: every envelope is signed by the device key; the
//     ingest Worker verifies against the pubkey it recorded at enrollment, and
//     the agent↔tenant binding is fixed at enrollment time.
//   - Replay: each signed message carries a fresh Nonce + Timestamp; the Worker
//     enforces a sliding-window nonce cache + monotonic timestamp.
//
// The private key never leaves the device and is never transmitted. Enrollment
// sends only the PUBLIC key plus a one-time enrollment token.
//
// Standard library only (crypto/ed25519 is stdlib).
package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

// Identity is an enrolled agent's persistent cryptographic identity. It is
// loaded from / persisted to disk and used to sign every outbound message.
type Identity struct {
	AgentID  string             `json:"agent_id"`
	TenantID string             `json:"tenant_id"`
	PubKey   []byte             `json:"pub_key"`  // ed25519 public key (32 bytes)
	PrivKey  ed25519.PrivateKey `json:"priv_key"` // 64 bytes; NEVER transmitted/logged

	// counter is a monotonic per-agent message counter, persisted alongside the
	// identity, that backs replay protection together with the timestamp+nonce.
	counter atomic.Uint64
}

// Generate creates a fresh, UNENROLLED identity with a new keypair. AgentID and
// TenantID are assigned by Enroll.
func Generate() (*Identity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("identity: keygen: %w", err)
	}
	return &Identity{PubKey: pub, PrivKey: priv}, nil
}

// Enrolled reports whether this identity has been bound to an agent/tenant.
func (id *Identity) Enrolled() bool {
	return id.AgentID != "" && id.TenantID != ""
}

// PubKeyB64 returns the base64 (std) encoded public key, the form sent to the
// control plane at enrollment and stored in agents.pubkey.
func (id *Identity) PubKeyB64() string {
	return base64.StdEncoding.EncodeToString(id.PubKey)
}

// Sign produces a detached Ed25519 signature over msg, base64 (std) encoded.
func (id *Identity) Sign(msg []byte) string {
	sig := ed25519.Sign(id.PrivKey, msg)
	return base64.StdEncoding.EncodeToString(sig)
}

// NextNonce returns a fresh, unique anti-replay nonce. It interleaves the
// monotonic counter with random bytes so the value is both unpredictable and
// guaranteed-unique per agent (the counter alone defeats duplicate nonces; the
// random suffix defeats prediction).
func (id *Identity) NextNonce() string {
	var buf [24]byte
	n := id.counter.Add(1)
	binary.BigEndian.PutUint64(buf[0:8], n)
	_, _ = rand.Read(buf[8:]) // best-effort entropy; counter already guarantees uniqueness
	return base64.RawURLEncoding.EncodeToString(buf[:])
}

// Counter returns the current monotonic counter value (for heartbeat/health).
func (id *Identity) Counter() uint64 { return id.counter.Load() }

// --- Persistence -----------------------------------------------------------

// persisted is the on-disk JSON shape. The private key is stored as raw bytes;
// at-rest protection (OS keystore / machine-bound encryption) is a wave0 TODO.
type persisted struct {
	AgentID  string `json:"agent_id"`
	TenantID string `json:"tenant_id"`
	PubKey   []byte `json:"pub_key"`
	PrivKey  []byte `json:"priv_key"`
	Counter  uint64 `json:"counter"`
}

// Load reads an identity from path. Returns os.ErrNotExist (wrapped) when the
// agent has not yet enrolled, so the caller can branch to Enroll.
func Load(path string) (*Identity, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p persisted
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("identity: corrupt identity file: %w", err)
	}
	if len(p.PrivKey) != ed25519.PrivateKeySize || len(p.PubKey) != ed25519.PublicKeySize {
		return nil, errors.New("identity: invalid key sizes in identity file")
	}
	id := &Identity{
		AgentID:  p.AgentID,
		TenantID: p.TenantID,
		PubKey:   p.PubKey,
		PrivKey:  ed25519.PrivateKey(p.PrivKey),
	}
	id.counter.Store(p.Counter)
	return id, nil
}

// Save persists the identity to path with 0600 perms (owner-only). The parent
// directory is created if needed. TODO(wave0): wrap the private key with a
// machine-bound key / OS keystore instead of plaintext-at-0600.
func (id *Identity) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	p := persisted{
		AgentID:  id.AgentID,
		TenantID: id.TenantID,
		PubKey:   id.PubKey,
		PrivKey:  id.PrivKey,
		Counter:  id.counter.Load(),
	}
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	// Write to a temp file then rename for atomicity.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// --- Enrollment ------------------------------------------------------------

// EnrollRequest is the body POSTed to POST /v1/enroll. It carries only the
// public key + one-time token + host hints; never the private key.
type EnrollRequest struct {
	Token     string `json:"token"`   // one-time enrollment token (redeemed server-side)
	PubKey    string `json:"pub_key"` // base64 std-encoded ed25519 public key
	Hostname  string `json:"hostname"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	AgentVer  string `json:"agent_ver"`
	Nonce     string `json:"nonce"` // anti-replay for the enroll call itself
	Timestamp int64  `json:"ts"`    // unix seconds
}

// EnrollResponse is the control plane's reply: the assigned identity binding.
type EnrollResponse struct {
	AgentID  string `json:"agent_id"`
	TenantID string `json:"tenant_id"`
}

// Apply records the server-assigned binding onto the identity. After Apply the
// identity is Enrolled() and should be persisted with Save.
func (id *Identity) Apply(resp EnrollResponse) error {
	if resp.AgentID == "" || resp.TenantID == "" {
		return errors.New("identity: enroll response missing agent/tenant id")
	}
	id.AgentID = resp.AgentID
	id.TenantID = resp.TenantID
	return nil
}

// BuildEnrollRequest constructs a self-signed enrollment request. The agent
// proves possession of the private key by signing the canonical request bytes;
// the server verifies that signature against the submitted public key before
// binding it, so a token thief who lacks the key cannot enroll a pubkey they
// don't control. The signature is returned separately (header X-Signature).
func (id *Identity) BuildEnrollRequest(token, hostname, osName, arch, agentVer string) (EnrollRequest, string, error) {
	req := EnrollRequest{
		Token:     token,
		PubKey:    id.PubKeyB64(),
		Hostname:  hostname,
		OS:        osName,
		Arch:      arch,
		AgentVer:  agentVer,
		Nonce:     id.NextNonce(),
		Timestamp: time.Now().UTC().Unix(),
	}
	canon, err := json.Marshal(req)
	if err != nil {
		return EnrollRequest{}, "", err
	}
	return req, id.Sign(canon), nil
}
