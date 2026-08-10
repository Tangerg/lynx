// Package teardown owns deadline-aware resource teardown for close operations
// that may not honor context cancellation.
package teardown

import (
	"context"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/completion"
)

// Step serializes teardown for one resource. A caller can time out while Step
// retains ownership of the in-flight operation; a later caller joins it instead
// of issuing concurrent teardown.
type Step struct {
	action func(context.Context) error

	mu       sync.Mutex
	complete bool
	active   *attemptState
}

type attemptState struct {
	done chan struct{}
	err  error
}

// Attempt is one immutable Step execution generation.
type Attempt struct {
	state *attemptState
}

// New returns a teardown step around action. A nil action is a completed no-op.
func New(action func(context.Context) error) *Step {
	return &Step{action: action}
}

// Shutdown starts action once, or joins its current attempt. Success makes
// future calls no-ops; failure remains retryable by a later call.
func (step *Step) Shutdown(ctx context.Context) error {
	return step.Begin(ctx).Wait(ctx)
}

// Begin starts a new generation, joins the active generation, or returns a
// completed no-op after successful teardown.
func (step *Step) Begin(ctx context.Context) *Attempt {
	if step == nil || step.action == nil {
		return completedAttempt(nil)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return completedAttempt(err)
	}
	step.mu.Lock()
	if step.complete {
		step.mu.Unlock()
		return completedAttempt(nil)
	}
	running := step.active
	if running == nil {
		running = &attemptState{done: make(chan struct{})}
		step.active = running
		go step.run(ctx, running)
	}
	step.mu.Unlock()
	return &Attempt{state: running}
}

// Wait joins this immutable attempt. The caller's deadline does not cancel or
// replace an execution already owned by Step.
func (attempt *Attempt) Wait(ctx context.Context) error {
	if attempt == nil || attempt.state == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := completion.Wait(ctx, attempt.state.done); err != nil {
		return err
	}
	return attempt.state.err
}

// Result reports whether the attempt completed and its terminal error.
func (attempt *Attempt) Result() (completed bool, err error) {
	if attempt == nil || attempt.state == nil {
		return true, nil
	}
	select {
	case <-attempt.state.done:
		return true, attempt.state.err
	default:
		return false, nil
	}
}

func completedAttempt(err error) *Attempt {
	state := &attemptState{done: make(chan struct{}), err: err}
	close(state.done)
	return &Attempt{state: state}
}

func (step *Step) run(ctx context.Context, running *attemptState) {
	err := step.action(ctx)
	step.mu.Lock()
	running.err = err
	if err == nil {
		step.complete = true
	}
	if step.active == running {
		step.active = nil
	}
	close(running.done)
	step.mu.Unlock()
}
