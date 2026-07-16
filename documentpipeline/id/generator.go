package id

import "context"

// Generator produces an identifier string. Implementations may derive
// the id from the input objects (deterministic, content-addressable —
// see [SHA256Generator]) or ignore them entirely (random — see
// [UUIDGenerator]).
type Generator interface {
	Generate(ctx context.Context, objects ...any) (string, error)
}
