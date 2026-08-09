// Package shutdown owns deadline-aware teardown steps for process resources
// whose underlying close operation may not accept a context.
package shutdown

import (
	"context"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/completion"
)

// Step serializes one teardown operation. If its action ignores cancellation, a
// caller still returns at its deadline while Step retains ownership of the
// in-flight operation. A later call joins that operation instead of issuing a
// concurrent second teardown.
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

// Attempt is one immutable Step run. It lets an owner start several
// teardown steps before joining them.
type Attempt struct {
	state *attemptState
}

// New returns a context-aware teardown step around action. A nil action is a
// no-op step, which keeps composition code free of nil-function branches.
func New(action func(context.Context) error) *Step {
	return &Step{action: action}
}

// Shutdown starts action once, or joins its current attempt. A successful
// action makes future calls no-ops; a failed action remains retryable. The
// caller's deadline is never extended by an action that ignores its context.
func (s *Step) Shutdown(ctx context.Context) error {
	return s.Begin(ctx).Wait(ctx)
}

// Begin starts one execution, joins the current execution, or returns an
// already completed no-op attempt. A completed failure is retried only by a
// later explicit Begin call.
func (s *Step) Begin(ctx context.Context) *Attempt {
	if s == nil || s.action == nil {
		return completedAttempt(nil)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return completedAttempt(err)
	}
	s.mu.Lock()
	if s.complete {
		s.mu.Unlock()
		return completedAttempt(nil)
	}
	running := s.active
	if running == nil {
		running = &attemptState{done: make(chan struct{})}
		s.active = running
		go s.run(ctx, running)
	}
	s.mu.Unlock()
	return &Attempt{state: running}
}

// Wait joins this immutable attempt. The caller's deadline does not cancel or
// replace an execution already owned by Step.
func (a *Attempt) Wait(ctx context.Context) error {
	if a == nil || a.state == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := completion.Wait(ctx, a.state.done); err != nil {
		return err
	}
	return a.state.err
}

// Result reports whether the attempt completed and returns its terminal error.
func (a *Attempt) Result() (completed bool, err error) {
	if a == nil || a.state == nil {
		return true, nil
	}
	select {
	case <-a.state.done:
		return true, a.state.err
	default:
		return false, nil
	}
}

func completedAttempt(err error) *Attempt {
	state := &attemptState{done: make(chan struct{}), err: err}
	close(state.done)
	return &Attempt{state: state}
}

func (s *Step) run(ctx context.Context, running *attemptState) {
	err := s.action(ctx)
	s.mu.Lock()
	running.err = err
	if err == nil {
		s.complete = true
	}
	if s.active == running {
		s.active = nil
	}
	close(running.done)
	s.mu.Unlock()
}
