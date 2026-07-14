package hitl

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/toolloop"
)

// InterruptError is the guard error any step returns to suspend the run for
// human input — the Go-ecosystem interrupt model. It carries a stable Key (the
// interrupt's identity, stable across the resuming re-run) and a
// user-facing Value (the payload surfaced to the client). It satisfies
// the target tool loop's explicit abort channel, so it propagates rather than
// becoming model-visible recoverable tool output. It also satisfies the agent
// action's interrupt contract. The action parks the run on
// Awaitable and surfaces Value. On resume the awaitable's handler records the
// human's response on the process blackboard, and [Interrupt] returns it at the
// original call site.
//
// This is the ONE mental model for every HITL flavor: tool-call approval
// (Interrupt[bool]), asking the user a question (Interrupt[string]), or any
// typed gather (Interrupt[T]).
type InterruptError struct {
	// Key identifies the interrupt so multiple interrupts in one turn don't
	// collide and so the recorded response matches across the re-run.
	Key string

	// Value is the user-facing payload (e.g. an approval prompt or a
	// question). Surfaced to the client verbatim; never persisted.
	Value any

	awaitable core.Awaitable
}

func (e *InterruptError) Error() string {
	return fmt.Sprintf("hitl.InterruptError: run interrupted for input (key %q)", e.Key)
}

// Unwrap exposes the target runner's explicit abort signal. Runner propagates
// the original InterruptError unchanged, allowing HandleInterrupt to recover
// the awaitable without a legacy boolean control-flow interface.
func (e *InterruptError) Unwrap() error {
	return &toolloop.AbortError{Err: errInterrupt}
}

var errInterrupt = errors.New("agent HITL interrupt")

// IsInterrupt reports whether err contains an agent HITL interrupt.
func IsInterrupt(err error) bool {
	_, ok := errors.AsType[*InterruptError](err)
	return ok
}

// Awaitable returns the parkable awaitable whose handler records the resume
// response on the blackboard. The action parks the process on it (see
// [HandleInterrupt]).
func (e *InterruptError) Awaitable() core.Awaitable { return e.awaitable }

// resumeSlotKey namespaces a per-interrupt resume value on the blackboard.
func resumeSlotKey(key string) string { return "hitl:resume:" + key }

// Interrupt is the universal HITL guard, written linearly at the call site:
//
//	answer, _, err := hitl.Interrupt[string](ctx, key, Question{Text: "..."})
//	if err != nil { return "", err } // bubbles up, parks the run
//	// answer holds the human's reply once resumed
//
// First pass: returns (zero, false, *InterruptError) carrying value — the
// run parks and value is surfaced to the client. On resume the human's
// response (recorded on the process blackboard by this interrupt's
// awaitable handler at ResumeProcess time) is returned as (resp, true,
// nil). key identifies the interrupt; reuse a stable key (e.g. derived from
// the tool name + arguments) so the value matches the same call site across
// the resuming re-run.
func Interrupt[R any](ctx context.Context, key string, value any) (resp R, resumed bool, err error) {
	var zero R
	proc := core.ProcessFrom(ctx)
	if proc == nil {
		return zero, false, errors.New("hitl.Interrupt: no process on context")
	}

	bb := proc.Blackboard()
	if v, ok := bb.Get(resumeSlotKey(key)); ok {
		if typed, ok := v.(R); ok {
			return typed, true, nil
		}
	}

	awaitable := NewTypedRequest(value, func(r R) core.ResponseImpact {
		bb.Set(resumeSlotKey(key), r)
		return core.ImpactUpdated
	})
	return zero, false, &InterruptError{Key: key, Value: value, awaitable: awaitable}
}

// HandleInterrupt parks the process on an interrupt's awaitable. The chat
// tool loop propagates an [*InterruptError] unchanged, so it surfaces to an
// action body either bare or wrapped (errors.As reaches it through Unwrap).
// When found it calls pc.AwaitInput and returns (ActionWaiting, true); the
// action body returns that status immediately. Otherwise returns (_, false)
// and the caller still handles err.
func HandleInterrupt(ctx context.Context, pc *core.ProcessContext, err error) (core.ActionStatus, bool) {
	ie, ok := errors.AsType[*InterruptError](err)
	if !ok {
		return 0, false
	}
	return pc.AwaitInput(ctx, ie.awaitable), true
}
