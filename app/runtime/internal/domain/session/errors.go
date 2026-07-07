package session

import "errors"

// ErrNotFound is returned by [Store.Get] / [Store.Fork] / [Store.Delete]
// when the supplied session id does not exist. Callers should errors.Is
// against this sentinel rather than string-matching.
var ErrNotFound = errors.New("session: not found")
