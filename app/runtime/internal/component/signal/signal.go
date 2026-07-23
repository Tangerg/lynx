// Package signal provides a synchronous, single-observer in-process signal.
// It carries no domain meaning: owning Application or adapter types define the
// payload, while the composition root connects producer and consumer.
package signal

import "sync"

// Signal forwards values to one observer. Publish is synchronous to preserve
// producer order; values published before an observer is installed are dropped.
// Safe for concurrent use.
type Signal[T any] struct {
	mu   sync.RWMutex
	sink func(T)
}

// Publish forwards value to the current observer when one is installed.
func (s *Signal[T]) Publish(value T) {
	s.mu.RLock()
	sink := s.sink
	s.mu.RUnlock()
	if sink != nil {
		sink(value)
	}
}

// Observe installs sink, replacing any earlier observer.
func (s *Signal[T]) Observe(sink func(T)) {
	s.mu.Lock()
	s.sink = sink
	s.mu.Unlock()
}
