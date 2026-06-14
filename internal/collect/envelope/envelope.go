// Package envelope builds and signs the versioned evidence Envelope that wraps
// every collector Record before it ships to the Keystone ingest endpoint.
//
// The Envelope type itself lives in internal/collector (the shared contract).
// This package owns the *construction + signing* of it: it produces the
// canonical byte sequence that gets signed, so the agent and the ingest Worker
// agree byte-for-byte on what the signature covers.
//
// Canonical signing form: we sign a compact, fixed-field, length-delimited
// concatenation of the security-relevant fields — NOT the JSON of the whole
// envelope (whose key order / whitespace is not guaranteed stable across
// encoders/languages). The Worker reconstructs the same bytes from the parsed
// fields and verifies. The payload is bound via its SHA-256 hash so a large
// payload need not be re-serialized identically to be authenticated.
//
// Standard library only.
package envelope

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jsdosanj/lookout/internal/collect/identity"
	"github.com/jsdosanj/lookout/internal/collector"
)

// Signer is the subset of *identity.Identity the envelope builder needs. Keeps
// the package testable without a full enrolled identity.
type Signer interface {
	Sign(msg []byte) string
	NextNonce() string
}

// Build assembles and signs an Envelope for a collector Record.
//
//   - agentID/tenantID bind the record to the enrolled agent (defense in depth;
//     the Worker also derives tenant from the verified agent identity).
//   - collectorID is the registry id of the producing collector (e.g.
//     "system_inventory"); it is recorded in collector_runs upstream.
//   - blobR2Key is the R2 object key when the payload was offloaded to R2; the
//     Envelope.Payload is then nil and the signature binds the blob's hash via
//     payloadHash. Pass "" and the marshaled payload for the inline case.
func Build(id *identity.Identity, collectorID string, rec collector.Record, blobR2Key string, payloadBytes []byte) (collector.Envelope, error) {
	if !id.Enrolled() {
		return collector.Envelope{}, fmt.Errorf("envelope: identity not enrolled")
	}
	env := collector.Envelope{
		SchemaVersion: collector.SchemaVersion,
		CollectorID:   collectorID,
		SchemaID:      rec.SchemaID,
		AgentID:       id.AgentID,
		TenantID:      id.TenantID,
		Timestamp:     time.Now().UTC(),
		Nonce:         id.NextNonce(),
		BlobR2Key:     blobR2Key,
	}
	// Inline payload only when it did not go to R2.
	if blobR2Key == "" {
		env.Payload = rec.Payload
	}
	env.Signature = id.Sign(SigningBytes(env, payloadBytes))
	return env, nil
}

// SigningBytes returns the canonical, language-neutral byte sequence the
// signature covers. Both the agent (here) and the ingest Worker MUST build this
// identically. Format: for each field, an 8-byte big-endian length prefix
// followed by the field bytes, in a FIXED order. payloadBytes is the exact
// serialized payload (inline) or the raw blob bytes (R2 case); we bind its
// SHA-256 so the signature authenticates content without requiring byte-stable
// re-serialization downstream.
func SigningBytes(env collector.Envelope, payloadBytes []byte) []byte {
	h := sha256.Sum256(payloadBytes)
	var verBuf [4]byte
	binary.BigEndian.PutUint32(verBuf[:], uint32(env.SchemaVersion))
	var tsBuf [8]byte
	binary.BigEndian.PutUint64(tsBuf[:], uint64(env.Timestamp.UnixNano()))

	fields := [][]byte{
		verBuf[:],
		[]byte(env.CollectorID),
		[]byte(env.SchemaID),
		[]byte(env.AgentID),
		[]byte(env.TenantID),
		tsBuf[:],
		[]byte(env.Nonce),
		[]byte(env.BlobR2Key),
		h[:],
	}

	// Compute total size for a single allocation.
	total := 0
	for _, f := range fields {
		total += 8 + len(f)
	}
	out := make([]byte, 0, total)
	var lp [8]byte
	for _, f := range fields {
		binary.BigEndian.PutUint64(lp[:], uint64(len(f)))
		out = append(out, lp[:]...)
		out = append(out, f...)
	}
	return out
}

// MarshalPayload serializes a Record's payload to the bytes that get hashed
// into the signature (inline case). Centralized so the spool, shipper, and
// envelope all agree on the exact bytes.
func MarshalPayload(rec collector.Record) ([]byte, error) {
	return json.Marshal(rec.Payload)
}
