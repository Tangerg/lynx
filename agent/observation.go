package agent

import (
	"context"
	"slices"
	"sync"
)

const defaultDeltaBuffer = 256

// EventListener observes ordered Framework facts. Panics are isolated from
// Process execution and never alter committed state. Implementations must
// return in bounded time and must not re-enter the observed Process.
type EventListener interface {
	// OnEvent receives one committed or attempted Framework fact in increasing
	// ProcessSequence for its Process. Different Processes may call the listener
	// concurrently. The callback must be bounded, must not re-enter the observed
	// Process, and has no veto or acknowledgment authority.
	OnEvent(ctx context.Context, event Event)
}

type EventListenerFunc func(ctx context.Context, event Event)

func (e EventListenerFunc) OnEvent(ctx context.Context, event Event) {
	e(ctx, event)
}

// DeltaListener observes best-effort Strategy streaming increments. Panics are
// isolated; slow listeners may cause bounded queue drops.
type DeltaListener interface {
	// OnDelta receives an accepted best-effort increment in queue order. Delivery
	// is sequential per listener but may lag Process execution; slow callbacks can
	// cause later increments to be dropped. The callback cannot affect execution.
	OnDelta(ctx context.Context, delta Delta)
}

type DeltaListenerFunc func(ctx context.Context, delta Delta)

func (d DeltaListenerFunc) OnDelta(ctx context.Context, delta Delta) {
	d(ctx, delta)
}

type observationBus struct {
	events []EventListener
	deltas []DeltaListener

	deltaMu     sync.RWMutex
	deltaQueue  chan deltaObservation
	deltaClosed bool
	deltaDone   chan struct{}
}

// deltaObservation is either one best-effort Delta or an ordering barrier. A
// barrier does not make dropped increments reliable; it only proves that every
// increment the bounded queue accepted before it has finished calling listeners.
type deltaObservation struct {
	delta   Delta
	barrier chan struct{}
}

func newObservationBus(events []EventListener, deltas []DeltaListener, capacity int) *observationBus {
	bus := &observationBus{
		events: slices.Clone(events),
		deltas: slices.Clone(deltas),
	}
	if len(bus.deltas) > 0 {
		bus.deltaQueue = make(chan deltaObservation, capacity)
		bus.deltaDone = make(chan struct{})
		go bus.deliverDeltas()
	}
	return bus
}

func (o *observationBus) publishEvent(ctx context.Context, event Event) {
	for _, listener := range o.events {
		callEventListener(ctx, listener, event)
	}
}

func callEventListener(ctx context.Context, listener EventListener, event Event) {
	defer func() { _ = recover() }()
	listener.OnEvent(ctx, event)
}

func (o *observationBus) offerDelta(delta Delta) bool {
	if len(o.deltas) == 0 {
		return true
	}
	o.deltaMu.RLock()
	defer o.deltaMu.RUnlock()
	if o.deltaClosed {
		return false
	}
	select {
	case o.deltaQueue <- deltaObservation{delta: delta}:
		return true
	default:
		return false
	}
}

func (o *observationBus) deliverDeltas() {
	defer close(o.deltaDone)
	for observation := range o.deltaQueue {
		if observation.barrier != nil {
			close(observation.barrier)
			continue
		}
		for _, listener := range o.deltas {
			callDeltaListener(listener, observation.delta)
		}
	}
}

func (o *observationBus) flushDeltas(ctx context.Context) error {
	if o.deltaQueue == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	barrier := make(chan struct{})
	o.deltaMu.RLock()
	if o.deltaClosed {
		o.deltaMu.RUnlock()
		return ErrEngineClosed
	}
	select {
	case o.deltaQueue <- deltaObservation{barrier: barrier}:
		o.deltaMu.RUnlock()
	case <-ctx.Done():
		o.deltaMu.RUnlock()
		return ctx.Err()
	}
	select {
	case <-barrier:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func callDeltaListener(listener DeltaListener, delta Delta) {
	defer func() { _ = recover() }()
	listener.OnDelta(context.Background(), delta)
}

func (o *observationBus) close() {
	if o.deltaQueue == nil {
		return
	}
	o.deltaMu.Lock()
	if !o.deltaClosed {
		o.deltaClosed = true
		close(o.deltaQueue)
	}
	o.deltaMu.Unlock()
	<-o.deltaDone
}
