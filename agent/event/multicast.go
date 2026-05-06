package event

import "sync"

// Listener is the subscriber surface. Implementations should be
// non-blocking; the multicast snapshots the listener slice under a
// short read lock and then dispatches outside the lock, so a slow
// listener doesn't block concurrent Add / Remove — but a slow listener
// still delays subsequent listeners on the same OnEvent call.
type Listener interface {
	OnEvent(e Event)
}

// ListenerFunc adapts a plain function into Listener.
type ListenerFunc func(e Event)

func (f ListenerFunc) OnEvent(e Event) { f(e) }

// Multicast is the concurrent-safe fan-out. Add/Remove may run while
// OnEvent is delivering — listeners are snapshotted under the lock and
// then invoked outside it.
type Multicast struct {
	mu        sync.RWMutex
	listeners []Listener
}

// NewMulticast returns an empty Multicast.
func NewMulticast() *Multicast { return &Multicast{} }

// Add appends a listener. Nil listeners are ignored to keep callers from
// having to nil-check.
func (m *Multicast) Add(l Listener) {
	if l == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners = append(m.listeners, l)
}

// Remove drops the supplied listener (by pointer identity). Listeners not
// present are silently ignored.
func (m *Multicast) Remove(l Listener) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, existing := range m.listeners {
		if existing == l {
			m.listeners = append(m.listeners[:i], m.listeners[i+1:]...)
			return
		}
	}
}

// OnEvent delivers to every registered listener, isolating each call so a
// panicking listener doesn't take down the rest. Listeners are snapshotted
// under the lock and then invoked outside it, so a slow listener can't
// block concurrent Add / Remove calls.
func (m *Multicast) OnEvent(e Event) {
	m.mu.RLock()
	listeners := make([]Listener, len(m.listeners))
	copy(listeners, m.listeners)
	m.mu.RUnlock()

	for _, listener := range listeners {
		safeDeliver(listener, e)
	}
}

// safeDeliver invokes the listener with a panic guard. Panicking
// listeners are a bug, but we don't want one to take down the whole
// process — production deployments can wire a recovering listener that
// reports to logs / metrics.
func safeDeliver(l Listener, e Event) {
	defer func() { _ = recover() }()
	l.OnEvent(e)
}
