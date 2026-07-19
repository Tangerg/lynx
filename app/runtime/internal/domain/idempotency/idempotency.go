// Package idempotency models durable JSON-RPC replay records. Keys are opaque
// client tokens; Fingerprint binds a key to exactly one method and parameter
// set, and Payload is the first JSON-RPC response envelope.
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

// Retention is the protocol-wide replay window for an idempotency key.
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
	// progress; a non-empty Payload is the completed response to replay.
	Claim(ctx context.Context, key, fingerprint string) (record Record, claimed bool, err error)
	// Complete stores the first response for a previously acquired claim.
	Complete(ctx context.Context, record Record) error
}
