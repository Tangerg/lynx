package agenttest

import (
	"context"
	"errors"
	"slices"
	"sync"

	agent "github.com/Tangerg/lynx/agent"
)

// ErrInvalidEventPredicate reports a missing AwaitEvent predicate.
var ErrInvalidEventPredicate = errors.New("agenttest: invalid event predicate")

// ObservationRecorder is a concurrency-safe EventListener and DeltaListener.
// Its zero value is ready for use.
type ObservationRecorder struct {
	mu      sync.Mutex
	events  []agent.Event
	deltas  []agent.Delta
	changed chan struct{}
}

// OnEvent records one immutable Framework event and wakes AwaitEvent callers.
func (recorder *ObservationRecorder) OnEvent(_ context.Context, event agent.Event) {
	if recorder == nil {
		return
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.events = append(recorder.events, event)
	recorder.notifyLocked()
}

// OnDelta records one immutable best-effort stream increment.
func (recorder *ObservationRecorder) OnDelta(_ context.Context, delta agent.Delta) {
	if recorder == nil {
		return
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.deltas = append(recorder.deltas, delta)
}

func (recorder *ObservationRecorder) notifyLocked() {
	if recorder.changed != nil {
		close(recorder.changed)
	}
	recorder.changed = make(chan struct{})
}

// Events returns recorded events in publication order.
func (recorder *ObservationRecorder) Events() []agent.Event {
	if recorder == nil {
		return nil
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return slices.Clone(recorder.events)
}

// Deltas returns recorded increments in delivery order.
func (recorder *ObservationRecorder) Deltas() []agent.Delta {
	if recorder == nil {
		return nil
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return slices.Clone(recorder.deltas)
}

// AwaitEvent returns the first recorded Event accepted by predicate. It waits
// for later events until ctx ends and never polls Process state.
func (recorder *ObservationRecorder) AwaitEvent(
	ctx context.Context,
	predicate func(agent.Event) bool,
) (agent.Event, error) {
	if recorder == nil || predicate == nil {
		return agent.Event{}, ErrInvalidEventPredicate
	}
	next := 0
	for {
		recorder.mu.Lock()
		batch := slices.Clone(recorder.events[next:])
		next = len(recorder.events)
		if recorder.changed == nil {
			recorder.changed = make(chan struct{})
		}
		changed := recorder.changed
		recorder.mu.Unlock()

		for _, event := range batch {
			if predicate(event) {
				return event, nil
			}
		}

		select {
		case <-ctx.Done():
			return agent.Event{}, ctx.Err()
		case <-changed:
		}
	}
}
