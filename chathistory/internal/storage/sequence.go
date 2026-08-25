package storage

import (
	"sync"
	"time"
)

// Sequence reserves monotonically increasing integer ranges. Its zero value
// is ready to use and remains monotonic when the wall clock moves backward.
type Sequence struct {
	mu   sync.Mutex
	last int64
}

// Reserve returns the first value in a range of count consecutive values.
func (sequence *Sequence) Reserve(count int) int64 {
	return sequence.reserveAt(time.Now().UnixNano(), count)
}

func (sequence *Sequence) reserveAt(candidate int64, count int) int64 {
	if count <= 0 {
		panic("chathistory: sequence count must be positive")
	}
	sequence.mu.Lock()
	defer sequence.mu.Unlock()
	first := candidate
	if first <= sequence.last {
		first = sequence.last + 1
	}
	sequence.last = first + int64(count) - 1
	return first
}
