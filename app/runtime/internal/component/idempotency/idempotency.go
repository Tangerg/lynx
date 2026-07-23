// Package idempotency provides durable request-result replay coordination.
//
// It intentionally deals in opaque keys, fingerprints, and bytes: transport
// adapters decide how to derive a request fingerprint and encode a response.
// Keeping this mechanism out of domain prevents JSON-RPC retention and replay
// policy from becoming business vocabulary.
package idempotency

import (
	"context"
	"errors"
	"time"
)

// ErrKeyConflict reports that a key is already bound to another request.
var ErrKeyConflict = errors.New("idempotency: key reused with different request")

// ErrClaimLost reports that completion no longer owns the reserved key.
var ErrClaimLost = errors.New("idempotency: claim is no longer available")

// Retention is the default replay window for an idempotency key.
const Retention = 24 * time.Hour

// Record is the durable state of one idempotent request.
type Record struct {
	Key         string
	Fingerprint string
	Payload     []byte
}

// Store atomically claims logical operations and persists their first result.
type Store interface {
	// Claim atomically reserves key for fingerprint. claimed=false returns the
	// existing claim: an empty Payload means its first execution is still in
	// progress; a non-empty Payload is the completed opaque result to replay.
	Claim(ctx context.Context, key, fingerprint string) (record Record, claimed bool, err error)
	// Complete stores the first result for a previously acquired claim.
	Complete(ctx context.Context, record Record) error
}
