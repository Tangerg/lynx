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

// NamedListener wraps a function as a [runtime.EventListener] — i.e.,
// a [core.Extension] (it has Name) that observes events published in its
// registration scope.
//
// It turns event consumption into a closure, which is what makes stream-style
// delivery possible without a listener type: capture a channel in fn and range
// it from a consumer goroutine. Delivery is synchronous with publication, so fn
// must not block the tick — a full channel is the closure's problem to decide.
//
// Use [NewNamedSubtreeListener] when a process-scoped listener must also observe
// descendants; fn owns any filtering by ProcessID. A nil fn makes OnEvent a
// no-op.
type NamedListener struct {
	name string
	fn   func(context.Context, Event)
}

// NamedSubtreeListener is the explicit descendant-observing variant of
// [NamedListener] when registered in ProcessOptions.Extensions.
type NamedSubtreeListener struct {
	*NamedListener
}

// NewNamedListener returns a NamedListener with the given name and
// callback. name should be non-empty and unique within the slice
// passed to the engine — the runtime rejects duplicate or empty
// extension names at registration time.
func NewNamedListener(name string, fn func(context.Context, Event)) *NamedListener {
	return &NamedListener{name: name, fn: fn}
}

// NewNamedSubtreeListener returns a listener whose process-scoped registration
// follows descendant processes created below that process.
func NewNamedSubtreeListener(name string, fn func(context.Context, Event)) *NamedSubtreeListener {
	return &NamedSubtreeListener{NamedListener: NewNamedListener(name, fn)}
}

// ObserveSubtree marks l as a runtime.SubtreeEventListener.
func (*NamedSubtreeListener) ObserveSubtree() {}

// Name implements [core.Extension].
func (l *NamedListener) Name() string { return l.name }

// OnEvent invokes fn; nil fn is a no-op.
func (l *NamedListener) OnEvent(ctx context.Context, event Event) {
	if l.fn != nil {
		l.fn(ctx, event)
	}
}

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

// Add subscribes listener and returns an idempotent cancellation function.
// Nil listeners are ignored.
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
		m.subscriptions = slices.Delete(m.subscriptions, index, index+1)
		return
	}
}

// OnEvent delivers to a stable snapshot of every registered listener. Each
// callback is panic-isolated so one faulty listener cannot suppress the rest.
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
		m.deliver(ctx, listener, cloneForDelivery(event))
	}
}

type deliveryCloner interface {
	cloneEvent() Event
}

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
