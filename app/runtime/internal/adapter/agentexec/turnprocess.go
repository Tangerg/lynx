package agentexec

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/suspension"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
)

// TurnCompletion is the immutable result of one process segment. It is the
// single source of truth for terminal mapping: status, typed output, and failure
// are captured after the Agent runtime has joined the segment.
type TurnCompletion struct {
	Status core.ProcessStatus
	Output *TurnOutput
	Err    error
}

// TurnProcess is the handle [Engine.StartTurn] returns. It exposes one typed
// completion boundary instead of separate status, output, and done signals.
//
// The interface lives in this package (not in the turn dispatcher) so
// test stubs can substitute a fake without standing up a full engine.
type TurnProcess interface {
	// ID is the underlying agent process id — surfaces to clients as
	// the turn handle so cancellation / resume requests route through
	// the runtime by process id.
	ID() string

	// Await joins the active segment and captures its immutable completion.
	// Exactly one owner calls Await for each initial or resumed segment.
	Await() TurnCompletion

	// Cancel marks the process [core.StatusKilled] via the engine.
	// The ongoing tick observes the status flip at its next checkpoint
	// and the run loop exits, delivering its error on Done().
	Cancel(ctx context.Context) error

	// Resume answers a HITL interrupt the process is parked on
	// (StatusWaiting) — a gated tool call or an ask_user / exit_plan_mode
	// question. It delivers the structured [interrupts.Resolution]
	// to the parked suspension and starts the next segment. Only valid after an
	// Await completion with [core.StatusWaiting].
	Resume(ctx context.Context, resolution interrupts.Resolution) error

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
	// A nil error is the complete release postcondition; on error the caller
	// retains ownership and must retry. NEVER call on a parked process, whose
	// snapshot must survive for resume.
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

func (p *turnProcess) ID() string { return p.process.ID() }

func (p *turnProcess) Await() TurnCompletion {
	if p == nil || p.process == nil || p.done == nil {
		return TurnCompletion{Err: errors.New("agentexec: await process: no active segment")}
	}
	runErr := <-p.done
	p.done = nil
	completion := TurnCompletion{Status: p.process.Status(), Err: runErr}
	if output, ok := core.Result[TurnOutput](p.process); ok {
		completion.Output = &output
	}
	if failure := p.process.Failure(); failure != nil {
		completion.Err = failure
	}
	return completion
}

func (p *turnProcess) Cancel(ctx context.Context) error {
	return p.engine.Kill(ctx, p.process.ID())
}

func (p *turnProcess) Resume(ctx context.Context, resolution interrupts.Resolution) error {
	parked := p.process.Suspension()
	if parked == nil {
		return fmt.Errorf("engine: process %s has no suspension", p.process.ID())
	}
	response, err := suspension.EncodeResolution(resolution)
	if err != nil {
		return err
	}
	if err := p.engine.Resume(p.process.ID(), parked.ID, response); err != nil {
		return err
	}
	done, err := p.engine.ContinueAsync(ctx, p.process.ID())
	if err != nil {
		return err
	}
	if done == nil {
		return errors.New("engine: continue process returned no completion boundary")
	}
	p.done = done
	return nil
}

func (p *turnProcess) Suspension() *agent.Suspension { return p.process.Suspension() }

func (p *turnProcess) Discard(ctx context.Context) error {
	if p == nil || p.process == nil || p.engine == nil {
		return errors.New("agentexec: discard process: incomplete turn process")
	}
	return p.engine.Discard(ctx, p.process.ID())
}
