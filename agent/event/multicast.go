package event

import (
	"context"
	"fmt"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var eventTracer = otel.Tracer("lynx/agent/event")

// Listener is the subscriber surface. Implementations should be
// non-blocking; the multicast snapshots the listener slice under a
// short read lock and then dispatches outside the lock, so a slow
// listener doesn't block concurrent Add / Remove — but a slow listener
// still delays subsequent listeners on the same OnEvent call.
type Listener interface {
	OnEvent(ctx context.Context, event Event)
}

// ListenerFunc adapts a plain function into Listener.
type ListenerFunc func(context.Context, Event)

func (f ListenerFunc) OnEvent(ctx context.Context, event Event) { f(ctx, event) }

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
func (m *Multicast) Add(listener Listener) {
	if listener == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners = append(m.listeners, listener)
}

// Remove drops the supplied listener (by pointer identity). Listeners not
// present are silently ignored.
func (m *Multicast) Remove(listener Listener) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for index, existing := range m.listeners {
		if existing == listener {
			m.listeners = append(m.listeners[:index], m.listeners[index+1:]...)
			return
		}
	}
}

// OnEvent delivers to every registered listener, isolating each call so a
// panicking listener doesn't take down the rest. Listeners are snapshotted
// under the lock and then invoked outside it, so a slow listener can't
// block concurrent Add / Remove calls.
func (m *Multicast) OnEvent(ctx context.Context, event Event) {
	if ctx == nil {
		ctx = context.Background()
	}

	m.mu.RLock()
	listeners := make([]Listener, len(m.listeners))
	copy(listeners, m.listeners)
	m.mu.RUnlock()

	for _, listener := range listeners {
		safeDeliver(ctx, listener, event)
	}
}

// safeDeliver invokes the listener with a panic guard. Panicking
// listeners are a bug, but a single panicking listener must not take
// down the whole process — delivery to the remaining listeners continues.
// The panic is not silent: it surfaces as a short error span so the failure is
// observable through the standard OTel pipeline.
func safeDeliver(ctx context.Context, listener Listener, event Event) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			return
		}
		_, span := eventTracer.Start(ctx, "agent.listener.panic",
			trace.WithSpanKind(trace.SpanKindInternal),
			trace.WithAttributes(
				attribute.String("agent.listener", fmt.Sprintf("%T", listener)),
				attribute.String("agent.event", fmt.Sprintf("%T", event)),
			),
		)
		err := fmt.Errorf("event listener panicked: %v", recovered)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
	}()
	listener.OnEvent(ctx, event)
}
