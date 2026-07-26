// Package shutdown owns retry-safe, deadline-aware teardown steps for process
// resources whose underlying Close operation may not accept a context.
package shutdown

import (
	"context"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/component/completion"
)

// Step serializes one teardown operation. If Action ignores cancellation, a
// caller still returns at its deadline while Step retains ownership of the
// in-flight operation. A later call joins that operation instead of issuing a
// concurrent second teardown.
type Step struct {
	action func(context.Context) error

	mu       sync.Mutex
	complete bool
	active   *attempt
}

type attempt struct {
	done chan struct{}
	err  error
}

// Attempt is one immutable Step execution. It lets an owner start several
// teardown steps before joining them, while Step still serializes retries.
type Attempt struct {
	state *attempt
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

// Begin starts action once, joins the current execution, or returns an already
// completed no-op attempt. A caller that joins a failing predecessor receives
// one fresh serialized attempt under its own context.
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
	joined := running != nil
	if running == nil {
		running = &attempt{done: make(chan struct{})}
		s.active = running
		go s.run(ctx, running)
	}
	s.mu.Unlock()
	if joined {
		result := &attempt{done: make(chan struct{})}
		go s.continueAfter(ctx, running, result)
		return &Attempt{state: result}
	}
	return &Attempt{state: running}
}

// continueAfter gives a caller that arrived during an older attempt its own
// serialized attempt if the joined operation fails. The later caller therefore
// never inherits an expired predecessor context as its final result.
func (s *Step) continueAfter(ctx context.Context, joined, result *attempt) {
	if err := completion.Wait(ctx, joined.done); err != nil {
		completeAttempt(result, err)
		return
	}
	if joined.err == nil {
		completeAttempt(result, nil)
		return
	}
	next := s.Begin(ctx)
	completeAttempt(result, next.Wait(ctx))
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

// Result returns the attempt error once it has completed.
func (a *Attempt) Result() (error, bool) {
	if a == nil || a.state == nil {
		return nil, true
	}
	select {
	case <-a.state.done:
		return a.state.err, true
	default:
		return nil, false
	}
}

func completedAttempt(err error) *Attempt {
	state := &attempt{done: make(chan struct{}), err: err}
	close(state.done)
	return &Attempt{state: state}
}

func completeAttempt(state *attempt, err error) {
	state.err = err
	close(state.done)
}

func (s *Step) run(ctx context.Context, running *attempt) {
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
