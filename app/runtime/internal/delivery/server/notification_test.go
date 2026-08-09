package server

import "sync"

type testNotification[T any] struct {
	mu   sync.RWMutex
	sink func(T)
}

func (n *testNotification[T]) Publish(value T) {
	n.mu.RLock()
	sink := n.sink
	n.mu.RUnlock()
	if sink != nil {
		sink(value)
	}
}

func (n *testNotification[T]) Observe(sink func(T)) {
	n.mu.Lock()
	n.sink = sink
	n.mu.Unlock()
}
