package session

import "errors"

// ErrNotFound is returned by [Service.Get] / [Service.Fork] / [Service.Delete]
// when the supplied session id does not exist. Callers should errors.Is
// against this sentinel rather than string-matching.
var ErrNotFound = errors.New("session: not found")

// ErrInUse is returned by [Service] operations that require exclusive
// access to a session (e.g. attempting a second turn while one is in
// flight). MVP enforces single-client-per-session.
var ErrInUse = errors.New("session: in use")
