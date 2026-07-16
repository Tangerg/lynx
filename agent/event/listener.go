package event

import "context"

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
