// Package teardown owns deadline-aware resource teardown for close operations
// that may not honor context cancellation.
package teardown

import (
	"context"
	"errors"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/completion"
)

// Step serializes teardown for one resource. A caller can time out while Step
// retains ownership of the in-flight operation; a later caller joins it instead
// of issuing concurrent teardown.
type Step struct {
	action        func(context.Context) error
	settleOnError bool

	mu            sync.Mutex
	settled       bool
	settlementErr error
	active        *attemptState
}

type attemptState struct {
	done chan struct{}
	err  error
}

// Attempt is one immutable Step execution generation.
type Attempt struct {
	state *attemptState
}

// Retryable returns a teardown step whose failed action remains unsettled. A
// later caller starts one new attempt; concurrent callers still join the same
// in-flight attempt.
func Retryable(action func(context.Context) error) *Step {
	return &Step{action: action}
}

// Terminal returns a teardown step whose action reaching a return statement is
// the resource's final state. Its error is a shutdown diagnostic, not evidence
// that replaying the same one-shot Close can make further progress.
func Terminal(action func(context.Context) error) *Step {
	return &Step{action: action, settleOnError: true}
}

// Shutdown starts or joins the current attempt and reports both resource
// settlement and its diagnostic. settled=false means the owner must retain the
// step and try again; settled=true means dependencies may now be released even
// when err is non-nil.
func (step *Step) Shutdown(ctx context.Context) (settled bool, err error) {
	attempt := step.Begin(ctx)
	waitErr := attempt.Wait(ctx)
	settled, err = step.Settlement()
	if settled {
		return settled, err
	}
	return false, waitErr
}

// Begin starts a new generation, joins the active generation, or returns a
// completed no-op after successful teardown.
func (step *Step) Begin(ctx context.Context) *Attempt {
	if step == nil || step.action == nil {
		return completedAttempt(nil)
	}
	if ctx == nil {
		return completedAttempt(errors.New("teardown: context is required"))
	}
	if err := ctx.Err(); err != nil {
		return completedAttempt(err)
	}
	step.mu.Lock()
	if step.settled {
		err := step.settlementErr
		step.mu.Unlock()
		return completedAttempt(err)
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
		return errors.New("teardown: wait context is required")
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

// Settlement returns the resource-level terminal state. It differs from
// [Attempt.Result]: a retryable action can finish with an error while the
// resource remains unsettled.
func (step *Step) Settlement() (settled bool, err error) {
	if step == nil || step.action == nil {
		return true, nil
	}
	step.mu.Lock()
	defer step.mu.Unlock()
	return step.settled, step.settlementErr
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
	if err == nil || step.settleOnError {
		step.settled = true
		step.settlementErr = err
	}
	if step.active == running {
		step.active = nil
	}
	close(running.done)
	step.mu.Unlock()
}
