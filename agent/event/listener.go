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

// Listener is the subscriber surface. Implementations should be non-blocking;
// Multicast delivers one event to listeners sequentially outside its lock.
type Listener interface {
	OnEvent(ctx context.Context, event Event)
}

// ListenerFunc adapts a plain function into Listener.
type ListenerFunc func(context.Context, Event)

func (f ListenerFunc) OnEvent(ctx context.Context, event Event) { f(ctx, event) }

// NamedListener wraps a function as a [runtime.EventListener] — i.e.,
// a [core.Extension] (it has Name) that observes every Event published
// through the multicast.
//
// Drop into Config.Extensions or ProcessOptions.Extensions; the
// runtime fans every event through fn. Use this when you want
// channel-backed / stream-style event consumption without writing a
// full Listener struct: capture a channel in the closure, push from
// fn, range from a consumer goroutine.
//
// Example — channel-backed streaming:
//
//	ch := make(chan event.Event, 64)
//	listener := event.NewNamedListener("sse-stream", func(_ context.Context, e event.Event) {
//	    select {
//	    case ch <- e:
//	    default:
//	        // drop on backpressure — caller-defined policy
//	    }
//	})
//	opts := core.ProcessOptions{Extensions: []core.Extension{listener}}
//	go func() {
//	    defer close(ch)
//	    _, _ = engine.Run(ctx, agent, bindings, opts)
//	}()
//	for e := range ch { sseSend(e) }
//
// The same listener can be registered engine-scoped
// (Config.Extensions) to observe every process; the fn closure
// is responsible for any filtering by ProcessID(). nil fn makes
// OnEvent a no-op — useful for tests that want to verify "registered
// but did nothing".
type NamedListener struct {
	name string
	fn   func(context.Context, Event)
}

// NewNamedListener returns a NamedListener with the given name and
// callback. name should be non-empty and unique within the slice
// passed to the engine — the runtime rejects duplicate or empty
// extension names at registration time.
func NewNamedListener(name string, fn func(context.Context, Event)) *NamedListener {
	return &NamedListener{name: name, fn: fn}
}

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
// already being delivered.
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
		m.subscriptions = append(m.subscriptions[:index], m.subscriptions[index+1:]...)
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
		m.deliver(ctx, listener, event)
	}
}

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
