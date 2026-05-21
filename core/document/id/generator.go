package id

import "context"

// Generator produces an identifier string. Implementations may derive
// the id from the input objects (deterministic, content-addressable —
// see [Sha256Generator]) or ignore them entirely (random — see
// [UUIDGenerator]).
type Generator interface {
	// Generate returns an id derived from the provided objects.
	Generate(ctx context.Context, objects ...any) (string, error)
}
