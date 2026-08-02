// Package sequence allocates monotonic ranges for ordered history writes.
package sequence

import (
	"sync"
	"time"
)

// Generator reserves non-overlapping, monotonically increasing int64 ranges.
// Its zero value is ready to use.
type Generator struct {
	mu   sync.Mutex
	last int64
}

// Reserve returns the first value in a range of count consecutive sequence
// numbers. count must be positive.
func (g *Generator) Reserve(count int) int64 {
	return g.reserveAt(time.Now().UnixNano(), count)
}

func (g *Generator) reserveAt(candidate int64, count int) int64 {
	if count <= 0 {
		panic("chathistory sequence: count must be positive")
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	first := candidate
	if first <= g.last {
		first = g.last + 1
	}
	g.last = first + int64(count) - 1
	return first
}
