package server

import "sync"

type testNotification[T any] struct {
	mu   sync.RWMutex
	sink func(T)
}

func (t *testNotification[T]) Publish(value T) {
	t.mu.RLock()
	sink := t.sink
	t.mu.RUnlock()
	if sink != nil {
		sink(value)
	}
}

func (t *testNotification[T]) Observe(sink func(T)) {
	t.mu.Lock()
	t.sink = sink
	t.mu.Unlock()
}
