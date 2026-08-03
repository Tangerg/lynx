package skills

import "errors"

// ErrNilSource means NewTools was called without a backing source.
var ErrNilSource = errors.New("skills: source must not be nil")
