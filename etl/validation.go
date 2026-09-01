package etl

import "errors"

// ErrNilDocument rejects absent content at transformation boundaries.
var ErrNilDocument = errors.New("etl: document must not be nil")
