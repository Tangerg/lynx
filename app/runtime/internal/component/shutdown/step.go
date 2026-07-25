// Package shutdown owns retry-safe, deadline-aware teardown steps for process
// resources whose underlying Close operation may not accept a context.
package shutdown

import (
	"context"
	"sync"
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

// New returns a context-aware teardown step around action. A nil action is a
// no-op step, which keeps composition code free of nil-function branches.
func New(action func(context.Context) error) *Step {
	return &Step{action: action}
}

// Shutdown starts action once, or joins its current attempt. A successful
// action makes future calls no-ops; a failed action remains retryable. The
// caller's deadline is never extended by an action that ignores its context.
func (s *Step) Shutdown(ctx context.Context) error {
	if s == nil || s.action == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	if s.complete {
		s.mu.Unlock()
		return nil
	}
	running := s.active
	if running == nil {
		running = &attempt{done: make(chan struct{})}
		s.active = running
		go s.run(ctx, running)
	}
	s.mu.Unlock()

	select {
	case <-running.done:
		return running.err
	case <-ctx.Done():
		return ctx.Err()
	}
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
