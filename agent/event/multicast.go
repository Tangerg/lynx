package event

import (
	"context"
	"fmt"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/agent/core"
)

// Listener is the subscriber surface. Implementations should be
// non-blocking; the multicast snapshots the listener slice under a
// short read lock and then dispatches outside the lock, so a slow
// listener doesn't block concurrent Add / Remove — but a slow listener
// still delays subsequent listeners on the same OnEvent call.
type Listener interface {
	OnEvent(ctx context.Context, e Event)
}

// ListenerFunc adapts a plain function into Listener.
type ListenerFunc func(context.Context, Event)

func (f ListenerFunc) OnEvent(ctx context.Context, e Event) { f(ctx, e) }

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
func (m *Multicast) OnEvent(ctx context.Context, e Event) {
	if ctx == nil {
		ctx = context.Background()
	}

	m.mu.RLock()
	listeners := make([]Listener, len(m.listeners))
	copy(listeners, m.listeners)
	m.mu.RUnlock()

	for _, listener := range listeners {
		safeDeliver(ctx, listener, e)
	}
}

// safeDeliver invokes the listener with a panic guard. Panicking
// listeners are a bug, but a single panicking listener must not take
// down the whole process — delivery to the remaining listeners continues.
// The panic is not silent: it surfaces as a short error span so the failure is
// observable through the standard OTel pipeline.
func safeDeliver(ctx context.Context, l Listener, e Event) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		_, span := core.AgentTracer().Start(ctx, "agent.listener.panic",
			trace.WithSpanKind(trace.SpanKindInternal),
			trace.WithAttributes(
				attribute.String("agent.listener", fmt.Sprintf("%T", l)),
				attribute.String("agent.event", fmt.Sprintf("%T", e)),
			),
		)
		err := fmt.Errorf("event listener panicked: %v", r)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
	}()
	l.OnEvent(ctx, e)
}
