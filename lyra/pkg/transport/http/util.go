package http

import (
	"strconv"
	"time"

	"github.com/Tangerg/lynx/lyra/pkg/transport"
)

// reference the transport package so the type alias below stays
// reachable through `transport.Message`. The actual import keeps
// every file in the package self-consistent.
var _ = (*transport.Message)(nil)

// formatSeq turns a monotone uint64 into the decimal-encoded
// eventId we hand to clients. strconv.FormatUint is well-optimised
// and zero-alloc beyond a small backing array.
func formatSeq(n uint64) string {
	return strconv.FormatUint(n, 10)
}

// nowFn is the time source used by the replay buffer — swappable in
// tests if we ever want to verify the 30s eviction explicitly.
var nowFn = time.Now
