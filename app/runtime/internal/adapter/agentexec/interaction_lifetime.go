package agentexec

import (
	"context"
	"errors"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
)

// interactionLifetime owns every goroutine and channel whose lifetime is the
// staged Interaction session. Its context is the only cancellation root below
// the executor, and release/finish are one-shot transitions joined through the
// worker groups.
type interactionLifetime struct {
	context     context.Context
	stop        context.CancelFunc
	events      chan runs.ExecutorEvent
	done        chan struct{}
	releasing   chan struct{}
	unknownWake chan struct{}
	stateWake   chan struct{}
	releaseOnce sync.Once
	finishOnce  sync.Once
	workers     sync.WaitGroup
	reconcilers sync.WaitGroup
}

func newInteractionLifetime(parent context.Context) interactionLifetime {
	lifetime, stop := context.WithCancel(parent)
	return interactionLifetime{
		context:     lifetime,
		stop:        stop,
		events:      make(chan runs.ExecutorEvent, interactionEventBuffer),
		done:        make(chan struct{}),
		releasing:   make(chan struct{}),
		unknownWake: make(chan struct{}, 1),
		stateWake:   make(chan struct{}, 1),
	}
}

func (lifetime *interactionLifetime) beginRelease() {
	lifetime.releaseOnce.Do(func() {
		lifetime.stop()
		close(lifetime.releasing)
	})
}

func (lifetime *interactionLifetime) offer(event runs.ExecutorEvent) bool {
	select {
	case lifetime.events <- event:
		return true
	default:
		return false
	}
}

func (lifetime *interactionLifetime) send(event runs.ExecutorEvent) bool {
	select {
	case lifetime.events <- event:
		return true
	case <-lifetime.releasing:
		return false
	}
}

func (lifetime *interactionLifetime) sendAuthoritative(
	ctx context.Context,
	event runs.ExecutorEvent,
) error {
	select {
	case lifetime.events <- event:
		return nil
	case <-lifetime.releasing:
		return errors.New("agentexec: execution released before authoritative fact commit")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (lifetime *interactionLifetime) bind(ctx context.Context) (context.Context, context.CancelFunc) {
	bound, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(lifetime.context, cancel)
	return bound, func() {
		stop()
		cancel()
	}
}

func (lifetime *interactionLifetime) wakeUnknown() {
	select {
	case lifetime.unknownWake <- struct{}{}:
	default:
	}
}

func (lifetime *interactionLifetime) wakeState() {
	select {
	case lifetime.stateWake <- struct{}{}:
	default:
	}
}
