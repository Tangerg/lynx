package trace

import "errors"

// ErrNotFound is returned when an id targets a non-existent trace.
var ErrNotFound = errors.New("trace: not found")
