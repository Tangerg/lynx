package etl

import "errors"

// ErrNilDocument reports a nil document at an ETL boundary.
var ErrNilDocument = errors.New("etl: document must not be nil")
