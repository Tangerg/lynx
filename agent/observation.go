package agent

import (
	"context"
	"sync"
)

const defaultDeltaBuffer = 256

// EventListener observes ordered Framework facts. Panics are isolated from
// Process execution and never alter committed state. Implementations must
// return in bounded time and must not re-enter the observed Process.
type EventListener interface {
	OnEvent(ctx context.Context, event Event)
}

// EventListenerFunc adapts a function to EventListener.
type EventListenerFunc func(ctx context.Context, event Event)

// OnEvent invokes listener.
func (listener EventListenerFunc) OnEvent(ctx context.Context, event Event) {
	listener(ctx, event)
}

// DeltaListener observes best-effort Strategy streaming increments. Panics are
// isolated; slow listeners may cause bounded queue drops.
type DeltaListener interface {
	OnDelta(ctx context.Context, delta Delta)
}

// DeltaListenerFunc adapts a function to DeltaListener.
type DeltaListenerFunc func(ctx context.Context, delta Delta)

// OnDelta invokes listener.
func (listener DeltaListenerFunc) OnDelta(ctx context.Context, delta Delta) {
	listener(ctx, delta)
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
		events: append([]EventListener(nil), events...),
		deltas: append([]DeltaListener(nil), deltas...),
	}
	if len(bus.deltas) > 0 {
		bus.deltaQueue = make(chan deltaObservation, capacity)
		bus.deltaDone = make(chan struct{})
		go bus.deliverDeltas()
	}
	return bus
}

func (bus *observationBus) publishEvent(ctx context.Context, event Event) {
	for _, listener := range bus.events {
		callEventListener(ctx, listener, event)
	}
}

func callEventListener(ctx context.Context, listener EventListener, event Event) {
	defer func() { _ = recover() }()
	listener.OnEvent(ctx, event)
}

func (bus *observationBus) offerDelta(delta Delta) bool {
	if len(bus.deltas) == 0 {
		return true
	}
	bus.deltaMu.RLock()
	defer bus.deltaMu.RUnlock()
	if bus.deltaClosed {
		return false
	}
	select {
	case bus.deltaQueue <- deltaObservation{delta: delta}:
		return true
	default:
		return false
	}
}

func (bus *observationBus) deliverDeltas() {
	defer close(bus.deltaDone)
	for observation := range bus.deltaQueue {
		if observation.barrier != nil {
			close(observation.barrier)
			continue
		}
		for _, listener := range bus.deltas {
			callDeltaListener(listener, observation.delta)
		}
	}
}

func (bus *observationBus) flushDeltas(ctx context.Context) error {
	if bus.deltaQueue == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	barrier := make(chan struct{})
	bus.deltaMu.RLock()
	if bus.deltaClosed {
		bus.deltaMu.RUnlock()
		return ErrEngineClosed
	}
	select {
	case bus.deltaQueue <- deltaObservation{barrier: barrier}:
		bus.deltaMu.RUnlock()
	case <-ctx.Done():
		bus.deltaMu.RUnlock()
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

func (bus *observationBus) close() {
	if bus.deltaQueue == nil {
		return
	}
	bus.deltaMu.Lock()
	if !bus.deltaClosed {
		bus.deltaClosed = true
		close(bus.deltaQueue)
	}
	bus.deltaMu.Unlock()
	<-bus.deltaDone
}
