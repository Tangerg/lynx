package bootstrap

import "sync"

// notificationRelay synchronously connects an in-process producer to one
// observer. It is composition wiring: values published before observation are
// intentionally dropped, and installing a new observer replaces the old one.
type notificationRelay[T any] struct {
	Publish func(T)
	Observe func(func(T))
}

func newNotificationRelay[T any]() notificationRelay[T] {
	var mu sync.RWMutex
	var sink func(T)
	return notificationRelay[T]{
		Publish: func(value T) {
			mu.RLock()
			current := sink
			mu.RUnlock()
			if current != nil {
				current(value)
			}
		},
		Observe: func(next func(T)) {
			mu.Lock()
			sink = next
			mu.Unlock()
		},
	}
}
