package agentexec

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
)

// TurnProcess is the handle [Engine.StartTurn] returns. It exposes
// the underlying [runtime.Process] lifecycle (status, failure,
// cancellation) plus a typed result extractor — turn.Dispatcher drives
// the turn off Done() and queries Status() to decide TurnEnd reason.
//
// The interface lives in this package (not in the turn dispatcher) so
// test stubs can substitute a fake without standing up a full engine.
type TurnProcess interface {
	// ID is the underlying agent process id — surfaces to clients as
	// the turn handle so cancellation / resume requests route through
	// the runtime by process id.
	ID() string

	// Status reports the current [core.ProcessStatus] —
	// Running while the action loop ticks, Completed / Failed /
	// Killed / Terminated when the run ends.
	Status() core.ProcessStatus

	// Done delivers the final error (or nil on success) once the
	// run loop exits. Buffered cap-1 so callers can receive after
	// the goroutine has already finished.
	Done() <-chan error

	// Output extracts the typed [TurnOutput] from the process
	// blackboard. Returns an error when the run produced no output
	// (status reflects the terminal cause).
	Output() (TurnOutput, error)

	// Cancel marks the process [core.StatusKilled] via the engine.
	// The ongoing tick observes the status flip at its next checkpoint
	// and the run loop exits, delivering its error on Done().
	Cancel(ctx context.Context) error

	// Resume answers a HITL interrupt the process is parked on
	// (StatusWaiting) — a gated tool call or an ask_user / exit_plan_mode
	// question. It delivers the structured [interrupts.Resolution]
	// to the parked suspension and continues the process, returning a fresh
	// Done channel for the resumed run. Only valid while Status is
	// [core.StatusWaiting].
	Resume(ctx context.Context, resolution interrupts.Resolution) (<-chan error, error)

	// Suspension returns the HITL request the process is parked
	// on while StatusWaiting (a gated tool call or an ask_user /
	// exit_plan_mode question), or nil when nothing is parked. Its
	// Prompt JSON is what the client renders to make the decision.
	Suspension() *agent.Suspension

	// Discard releases a TERMINATED process: it removes the process from the
	// engine registry and deletes its persisted snapshot. With a ProcessStore
	// wired the runtime auto-snapshots every tick — including terminal
	// completion — but that snapshot only matters while the process is PARKED
	// awaiting HITL resume; once the turn reaches a terminal state it is dead
	// weight, and left behind it accumulates one orphaned snapshot row per run.
	// Cleanup failures don't rewrite the already-finished turn outcome, but are
	// returned so the owning turn span can retain them. Call exactly once at
	// terminal teardown — NEVER on a parked process, whose snapshot must survive
	// for resume.
	Discard(ctx context.Context) error
}

// turnProcess is the canonical [TurnProcess] backed by a real
// [runtime.Process]. It is package-private, so retaining the concrete Agent
// runtime keeps lifecycle commands inside this execution adapter.
type turnProcess struct {
	process *runtime.Process
	done    <-chan error
	engine  *runtime.Engine
}

func (p *turnProcess) ID() string                 { return p.process.ID() }
func (p *turnProcess) Status() core.ProcessStatus { return p.process.Status() }
func (p *turnProcess) Done() <-chan error         { return p.done }
func (p *turnProcess) Cancel(ctx context.Context) error {
	return p.engine.Kill(ctx, p.process.ID())
}

func (p *turnProcess) Resume(ctx context.Context, resolution interrupts.Resolution) (<-chan error, error) {
	suspension := p.process.Suspension()
	if suspension == nil {
		return nil, fmt.Errorf("engine: process %s has no suspension", p.process.ID())
	}
	if err := p.engine.Resume(p.process.ID(), suspension.ID, resolution); err != nil {
		return nil, err
	}
	return p.engine.ContinueAsync(ctx, p.process.ID())
}

func (p *turnProcess) Suspension() *agent.Suspension { return p.process.Suspension() }

func (p *turnProcess) Discard(ctx context.Context) error {
	if p == nil || p.process == nil || p.engine == nil {
		return errors.New("agentexec: discard process: incomplete turn process")
	}
	return p.engine.Discard(ctx, p.process.ID())
}

func (p *turnProcess) Output() (TurnOutput, error) {
	output, ok := core.Result[TurnOutput](p.process)
	if ok {
		return output, nil
	}
	// Preserve the process failure's error chain when there is one (%w);
	// a bare %w on a nil failure would format as "%!w(<nil>)".
	if failure := p.process.Failure(); failure != nil {
		return TurnOutput{}, fmt.Errorf("engine: no TurnOutput produced (status=%s): %w", p.process.Status(), failure)
	}
	return TurnOutput{}, fmt.Errorf("engine: no TurnOutput produced (status=%s)", p.process.Status())
}
