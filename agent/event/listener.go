package event

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/agent/internal/nilvalue"
	"github.com/Tangerg/lynx/agent/internal/panicerr"
)

const (
	eventTracerName   = "lynx/agent/event"
	spanListenerPanic = "agent.listener.panic"
	attrListenerType  = "agent.listener"
	attrEventType     = "agent.event"
)

var eventTracer = otel.Tracer(eventTracerName)

// Listener is the subscriber surface. Implementations should be non-blocking.
// One Multicast delivery visits its listener snapshot sequentially, but separate
// publishers may call the same Listener concurrently; implementations own
// synchronization and backpressure.
type Listener interface {
	OnEvent(ctx context.Context, event Event)
}

// ListenerFunc adapts a plain function into Listener.
type ListenerFunc func(context.Context, Event)

func (f ListenerFunc) OnEvent(ctx context.Context, event Event) { f(ctx, event) }

// Multicast is a concurrent-safe fan-out. A delivery uses the subscription
// snapshot captured when it began, so cancellation never interrupts an event
// already being delivered. Event types carrying mutable protocol containers
// provide an isolated value for each listener.
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

// Subscribe registers listener and returns an idempotent unsubscribe function.
// Nil listeners, including interfaces holding a typed nil, are ignored.
func (m *Multicast) Subscribe(listener Listener) func() {
	if nilvalue.Is(listener) {
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
		m.subscriptions = slices.Delete(m.subscriptions, index, index+1)
		return
	}
}

// OnEvent delivers a non-nil event to a stable snapshot of every registered
// listener. Each callback is panic-isolated so one faulty listener cannot
// suppress the rest.
func (m *Multicast) OnEvent(ctx context.Context, event Event) {
	if nilvalue.Is(event) {
		return
	}
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
		m.deliver(ctx, listener, cloneForDelivery(event))
	}
}

type deliveryCloner interface {
	cloneEvent() Event
}

// Delivery clones an event only for the types that opt in by type assertion, so
// a renamed or re-signatured cloneEvent would hand every listener the same
// mutable value instead of failing the build.
var (
	_ deliveryCloner = ProcessWaiting{}
	_ deliveryCloner = InteractionBoundary{}
)

func cloneForDelivery(value Event) Event {
	if cloner, ok := value.(deliveryCloner); ok {
		return cloner.cloneEvent()
	}
	return value
}

func (m *Multicast) deliver(ctx context.Context, listener Listener, event Event) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			return
		}
		_, span := eventTracer.Start(ctx, spanListenerPanic,
			trace.WithSpanKind(trace.SpanKindInternal),
			trace.WithAttributes(
				attribute.String(attrListenerType, fmt.Sprintf("%T", listener)),
				attribute.String(attrEventType, fmt.Sprintf("%T", event)),
			),
		)
		err := panicerr.New("event listener panicked", recovered)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
	}()
	listener.OnEvent(ctx, event)
}
