package runtime

import (
	"errors"
)

// maxChildDepth limits recursive delegation. A root process has depth zero.
const maxChildDepth = 8

// ErrChildDepth reports that a child would exceed the delegation depth limit.
var ErrChildDepth = errors.New("run child: max delegation depth exceeded")
