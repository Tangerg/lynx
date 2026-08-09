// Package notification bridges one in-process producer to one observer without
// assigning product meaning to the values it relays.
package notification

import "sync"

// Relay synchronously forwards values to one observer. Values published before
// an observer is installed are dropped. Relay is safe for concurrent use.
type Relay[T any] struct {
	mu   sync.RWMutex
	sink func(T)
}

// Publish forwards value to the current observer when one is installed.
func (r *Relay[T]) Publish(value T) {
	r.mu.RLock()
	sink := r.sink
	r.mu.RUnlock()
	if sink != nil {
		sink(value)
	}
}

// Observe installs sink, replacing any earlier observer.
func (r *Relay[T]) Observe(sink func(T)) {
	r.mu.Lock()
	r.sink = sink
	r.mu.Unlock()
}
