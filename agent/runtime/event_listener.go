package runtime

import (
	"context"

	"github.com/Tangerg/lynx/agent/event"
)

// NamedEventListener adapts a callback to the runtime's named extension
// contract. Delivery is synchronous with publication, and separate publishers
// may invoke the same callback concurrently.
type NamedEventListener struct {
	name string
	fn   func(context.Context, event.Event)
}

// NamedSubtreeEventListener is the descendant-observing variant of
// [NamedEventListener] for a process-scoped registration.
type NamedSubtreeEventListener struct {
	*NamedEventListener
}

// NewEventListener returns a named runtime event listener. The runtime rejects
// an empty or duplicate name when the listener is registered. A nil callback
// makes event delivery a no-op.
func NewEventListener(name string, fn func(context.Context, event.Event)) *NamedEventListener {
	return &NamedEventListener{name: name, fn: fn}
}

// NewSubtreeEventListener returns a process-scoped listener whose registration
// follows descendant processes.
func NewSubtreeEventListener(name string, fn func(context.Context, event.Event)) *NamedSubtreeEventListener {
	return &NamedSubtreeEventListener{NamedEventListener: NewEventListener(name, fn)}
}

// ObserveSubtree marks the listener as a [SubtreeEventListener].
func (*NamedSubtreeEventListener) ObserveSubtree() {}

// Name returns the frozen extension registration name.
func (l *NamedEventListener) Name() string { return l.name }

// OnEvent invokes the callback when one was supplied.
func (l *NamedEventListener) OnEvent(ctx context.Context, published event.Event) {
	if l.fn != nil {
		l.fn(ctx, published)
	}
}
