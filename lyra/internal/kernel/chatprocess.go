package kernel

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/lyra/internal/domain/interrupts"
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
	Cancel() error

	// Resume answers a HITL interrupt the process is parked on
	// (StatusWaiting) — a gated tool call or an ask_user / exit_plan_mode
	// question. It delivers the structured [interrupts.Resolution]
	// to the parked awaitable and continues the process, returning a fresh
	// Done channel for the resumed run. Only valid while Status is
	// [core.StatusWaiting].
	Resume(ctx context.Context, resolution interrupts.Resolution) (<-chan error, error)

	// PendingAwaitable returns the HITL request the process is parked
	// on while StatusWaiting (a gated tool call or an ask_user /
	// exit_plan_mode question), or nil when nothing is parked. Its
	// PromptAny() payload is what the client renders to make the decision.
	PendingAwaitable() core.Awaitable

	// Discard releases a TERMINATED process: it removes the process from the
	// platform registry and deletes its persisted snapshot. With a ProcessStore
	// wired the runtime auto-snapshots every tick — including terminal
	// completion — but that snapshot only matters while the process is PARKED
	// awaiting HITL resume; once the turn reaches a terminal state it is dead
	// weight, and left behind it accumulates one orphaned snapshot row per run.
	// Best-effort: cleanup failures don't affect the already-finished turn. Call
	// exactly once at terminal teardown — NEVER on a parked process, whose
	// snapshot must survive for resume.
	Discard(ctx context.Context)
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
func (cp *chatProcess) Cancel() error {
	return cp.platform.KillProcess(cp.proc.ID())
}

func (cp *chatProcess) Resume(ctx context.Context, resolution interrupts.Resolution) (<-chan error, error) {
	if _, err := cp.platform.ResumeProcess(cp.proc.ID(), resolution); err != nil {
		return nil, err
	}
	return cp.platform.ContinueProcessAsync(ctx, cp.proc.ID()), nil
}

func (cp *chatProcess) PendingAwaitable() core.Awaitable { return cp.proc.PendingAwaitable() }

func (cp *chatProcess) Discard(ctx context.Context) {
	id := cp.proc.ID()
	_ = cp.platform.RemoveProcess(id) // free the in-memory registry entry
	if store := cp.platform.ProcessStore(); store != nil {
		_ = store.Delete(ctx, id) // drop the persisted (terminal) snapshot
	}
}

func (cp *chatProcess) Output() (ChatOutput, error) {
	out, ok := core.ResultOfType[ChatOutput](cp.proc)
	if ok {
		return out, nil
	}
	// Preserve the process failure's error chain when there is one (%w);
	// a bare %w on a nil failure would format as "%!w(<nil>)".
	if failure := cp.proc.Failure(); failure != nil {
		return ChatOutput{}, fmt.Errorf("engine: no ChatOutput produced (status=%s): %w", cp.proc.Status(), failure)
	}
	return ChatOutput{}, fmt.Errorf("engine: no ChatOutput produced (status=%s)", cp.proc.Status())
}
