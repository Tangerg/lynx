package hitl

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/toolloop"
	coremodel "github.com/Tangerg/lynx/core/model"
)

// InterruptError is the guard error any step returns to suspend the run for
// human input — the Go-ecosystem interrupt model. It carries a stable Key (the
// interrupt's identity, stable across the resuming re-run) and a
// user-facing Value (the payload surfaced to the client). It satisfies
// [toolloop.Halt] with Abort() == false, so the tool loop exits immediately
// on it and propagates it (rather than feeding it back). It also satisfies
// [coremodel.ControlFlowError], so shared observability treats the pause as
// expected control flow rather than a failed model operation. The agent action
// parks the run on Awaitable and surfaces Value. On resume the awaitable's
// handler records the human's response on the process blackboard, and
// [Interrupt] returns it at the original call site.
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

var _ toolloop.Halt = (*InterruptError)(nil)
var _ coremodel.ControlFlowError = (*InterruptError)(nil)

func (e *InterruptError) Error() string {
	return fmt.Sprintf("hitl.InterruptError: run interrupted for input (key %q)", e.Key)
}

// Abort implements [toolloop.Halt]: an InterruptError HALTS the tool loop
// (propagated unchanged, never fed back to the model as a recoverable result),
// and Abort() == false marks it a HITL suspension — the run is expected to
// resume, not fail.
func (e *InterruptError) Abort() bool { return false }

// ControlFlow marks this interrupt as expected suspension rather than a failed
// model operation for shared observability.
func (e *InterruptError) ControlFlow() bool { return true }

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
func HandleInterrupt(pc *core.ProcessContext, err error) (core.ActionStatus, bool) {
	ie, ok := errors.AsType[*InterruptError](err)
	if !ok {
		return 0, false
	}
	return pc.AwaitInput(ie.awaitable), true
}
