package session

import "errors"

// ErrNotFound is returned when a session lookup, fork, or delete targets an id
// that does not exist. Callers should errors.Is against this sentinel rather
// than string-matching.
var ErrNotFound = errors.New("session: not found")
