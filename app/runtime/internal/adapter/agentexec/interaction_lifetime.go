package agentexec

import (
	"context"
	"errors"
	"sync"

	"github.com/Tangerg/scope/app/runtime/internal/application/runs"
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

func (i *interactionLifetime) beginRelease() {
	i.releaseOnce.Do(func() {
		i.stop()
		close(i.releasing)
	})
}

func (i *interactionLifetime) offer(event runs.ExecutorEvent) bool {
	select {
	case i.events <- event:
		return true
	default:
		return false
	}
}

func (i *interactionLifetime) send(event runs.ExecutorEvent) bool {
	select {
	case i.events <- event:
		return true
	case <-i.releasing:
		return false
	}
}

func (i *interactionLifetime) sendAuthoritative(
	ctx context.Context,
	event runs.ExecutorEvent,
) error {
	select {
	case i.events <- event:
		return nil
	case <-i.releasing:
		return errors.New("agentexec: execution released before authoritative fact commit")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (i *interactionLifetime) bind(ctx context.Context) (context.Context, context.CancelFunc) {
	bound, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(i.context, cancel)
	return bound, func() {
		stop()
		cancel()
	}
}

func (i *interactionLifetime) wakeUnknown() {
	select {
	case i.unknownWake <- struct{}{}:
	default:
	}
}

func (i *interactionLifetime) wakeState() {
	select {
	case i.stateWake <- struct{}{}:
	default:
	}
}
