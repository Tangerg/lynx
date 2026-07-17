package event

import (
	"context"
	"fmt"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/agent/internal/panicerr"
)

var eventTracer = otel.Tracer("lynx/agent/event")

// Listener is the subscriber surface. Implementations should be
// non-blocking; the multicast snapshots its subscriptions under a
// short read lock and then dispatches outside the lock, so a slow
// listener doesn't block concurrent subscription changes — but it
// still delays subsequent listeners on the same OnEvent call.
type Listener interface {
	OnEvent(ctx context.Context, event Event)
}

// ListenerFunc adapts a plain function into Listener.
type ListenerFunc func(context.Context, Event)

func (f ListenerFunc) OnEvent(ctx context.Context, event Event) { f(ctx, event) }

// Multicast is a concurrent-safe fan-out. Subscriptions may be added or
// canceled while OnEvent is delivering. A delivery uses the subscription
// snapshot captured when it began, so canceling does not interrupt an
// already-started delivery.
type Multicast struct {
	mu            sync.RWMutex
	nextID        uint64
	subscriptions []subscription
}

type subscription struct {
	id       uint64
	listener Listener
}

// NewMulticast returns an empty Multicast.
func NewMulticast() *Multicast { return &Multicast{} }

// Add subscribes listener and returns an idempotent function that cancels the
// subscription. Nil listeners are ignored and receive a no-op cancel function.
func (m *Multicast) Add(listener Listener) func() {
	if listener == nil {
		return func() {}
	}

	m.mu.Lock()
	m.nextID++
	id := m.nextID
	m.subscriptions = append(m.subscriptions, subscription{id: id, listener: listener})
	m.mu.Unlock()

	return func() {
		m.remove(id)
	}
}

func (m *Multicast) remove(id uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for index, current := range m.subscriptions {
		if current.id != id {
			continue
		}
		m.subscriptions = append(m.subscriptions[:index], m.subscriptions[index+1:]...)
		return
	}
}

// OnEvent delivers to every registered listener, isolating each call so a
// panicking listener doesn't take down the rest. Subscriptions are snapshotted
// under the lock and then invoked outside it, so a slow listener can't
// block concurrent subscription changes.
func (m *Multicast) OnEvent(ctx context.Context, event Event) {
	if ctx == nil {
		ctx = context.Background()
	}

	m.mu.RLock()
	listeners := make([]Listener, len(m.subscriptions))
	for index, current := range m.subscriptions {
		listeners[index] = current.listener
	}
	m.mu.RUnlock()

	for _, listener := range listeners {
		m.deliver(ctx, listener, event)
	}
}

// deliver invokes the listener with a panic guard. Panicking
// listeners are a bug, but a single panicking listener must not take
// down the whole process — delivery to the remaining listeners continues.
// The panic is not silent: it surfaces as a short error span so the failure is
// observable through the standard OTel pipeline.
func (m *Multicast) deliver(ctx context.Context, listener Listener, event Event) {
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
		err := panicerr.New("event listener panicked", recovered)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
	}()
	listener.OnEvent(ctx, event)
}
