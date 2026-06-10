package engine

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

// ChatProcess is the handle [Engine.StartChat] returns. It exposes
// the underlying [runtime.AgentProcess] lifecycle (status, failure,
// cancellation) plus a typed result extractor — chat.Service drives
// the turn off Done() and queries Status() to decide TurnEnd reason.
//
// The interface lives in this package (not in the chat service) so
// test stubs can substitute a fake without standing up a full platform.
type ChatProcess interface {
	// ID is the underlying agent process id — surfaces to clients as
	// the turn handle so cancellation / resume requests route through
	// the runtime by process id.
	ID() string

	// Status reports the current [core.AgentProcessStatus] —
	// Running while the action loop ticks, Completed / Failed /
	// Killed / Terminated when the run ends.
	Status() core.AgentProcessStatus

	// Done delivers the final error (or nil on success) once the
	// run loop exits. Buffered cap-1 so callers can receive after
	// the goroutine has already finished.
	Done() <-chan error

	// Output extracts the typed [ChatOutput] from the process
	// blackboard. Returns an error when the run produced no output
	// (status reflects the terminal cause).
	Output() (ChatOutput, error)

	// Cancel marks the process [core.StatusKilled] via the platform.
	// The ongoing tick observes the status flip at its next checkpoint
	// and the run loop exits, delivering its error on Done().
	Cancel(reason string) error

	// Resume answers a HITL interrupt the process is parked on
	// (StatusWaiting) — a plan-mode plan, a gated tool call, or an
	// ask_user question. It delivers the structured [InterruptResolution]
	// to the parked awaitable and continues the process, returning a fresh
	// Done channel for the resumed run. Only valid while Status is
	// [core.StatusWaiting].
	Resume(ctx context.Context, resolution InterruptResolution) (<-chan error, error)

	// PendingAwaitable returns the HITL request the process is parked
	// on while StatusWaiting (plan confirmation or tool-approval
	// confirmation), or nil when nothing is parked. Its PromptAny()
	// payload is what the client renders to make the decision.
	PendingAwaitable() core.Awaitable
}

// chatProcess is the canonical [ChatProcess] backed by a real
// [runtime.AgentProcess]. Platform reference is held so Cancel can
// invoke [runtime.Platform.KillProcess] without callers reaching
// into engine internals.
type chatProcess struct {
	proc     *runtime.AgentProcess
	done     <-chan error
	platform *runtime.Platform
}

func (cp *chatProcess) ID() string                      { return cp.proc.ID() }
func (cp *chatProcess) Status() core.AgentProcessStatus { return cp.proc.Status() }
func (cp *chatProcess) Done() <-chan error              { return cp.done }
func (cp *chatProcess) Cancel(reason string) error {
	_ = reason
	return cp.platform.KillProcess(cp.proc.ID())
}

func (cp *chatProcess) Resume(ctx context.Context, resolution InterruptResolution) (<-chan error, error) {
	if _, err := cp.platform.ResumeProcess(cp.proc.ID(), resolution); err != nil {
		return nil, err
	}
	return cp.platform.ContinueProcessAsync(ctx, cp.proc.ID()), nil
}

func (cp *chatProcess) PendingAwaitable() core.Awaitable { return cp.proc.PendingAwaitable() }

func (cp *chatProcess) Output() (ChatOutput, error) {
	out, ok := core.ResultOfType[ChatOutput](cp.proc)
	if !ok {
		return ChatOutput{}, fmt.Errorf("engine: no ChatOutput produced; status=%s failure=%v", cp.proc.Status(), cp.proc.Failure())
	}
	return out, nil
}
