package agentexec

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/suspension"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
)

// TurnCompletion is the typed application projection of one Agent runtime
// segment completion.
type TurnCompletion struct {
	Status    core.ProcessStatus
	Output    TurnOutput
	HasOutput bool
	Failure   error
	Err       error
}

func (c TurnCompletion) Error() error {
	if c.Failure == nil {
		return c.Err
	}
	if c.Err == nil {
		return c.Failure
	}
	if errors.Is(c.Err, c.Failure) {
		return c.Err
	}
	if errors.Is(c.Failure, c.Err) {
		return c.Failure
	}
	return errors.Join(c.Failure, c.Err)
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

	// Cancel marks the process [core.StatusKilled] via the engine. The ongoing
	// tick observes the status flip at its next checkpoint and completes its
	// active segment.
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
	// engine registry and deletes any persisted waiting snapshot. Only a parked
	// process needs a restart checkpoint; terminal segments are never persisted.
	// A failure retains the terminal runtime tree so cleanup can retry the same
	// deletion. Calling Discard on a non-terminal process fails before storage
	// is touched.
	Discard(ctx context.Context) error
}

// turnProcess is the canonical [TurnProcess] backed by a real
// [runtime.Process]. It is package-private, so retaining the concrete Agent
// runtime keeps lifecycle commands inside this execution adapter.
type turnProcess struct {
	process  *runtime.Process
	segment  *runtime.Segment
	owner    *Engine
	scope    execution.TurnScope
	runCtx   context.Context
	usage    *usageLedger
	provider string
	budget   accounting.Budget
}

const checkpointCommitTimeout = 10 * time.Second

func (p *turnProcess) ID() string { return p.process.ID() }

func (p *turnProcess) Await() TurnCompletion {
	if p == nil || p.process == nil || p.segment == nil {
		return TurnCompletion{Err: errors.New("agentexec: await process: no active segment")}
	}
	segmentCompletion, err := p.segment.Await(context.Background())
	if err != nil {
		return TurnCompletion{Err: err}
	}
	p.segment = nil
	completion := TurnCompletion{
		Status:  segmentCompletion.Status,
		Failure: segmentCompletion.Failure,
		Err:     segmentCompletion.Err,
	}
	if output, ok := runtime.CompletionResult[TurnOutput](segmentCompletion); ok {
		completion.Output = output
		completion.HasOutput = true
	}
	completion.Err = errors.Join(completion.Err, p.persistWaitingCheckpoint(completion.Status))
	return completion
}

func (p *turnProcess) Cancel(ctx context.Context) error {
	if p == nil || p.owner == nil || p.owner.runtime == nil || p.process == nil {
		return errors.New("agentexec: cancel process: incomplete turn process")
	}
	return p.owner.runtime.Kill(ctx, p.process.ID())
}

func (p *turnProcess) Resume(ctx context.Context, resolution interrupts.Resolution) error {
	if p == nil || p.process == nil || p.owner == nil || p.owner.runtime == nil {
		return errors.New("agentexec: resume process: incomplete turn process")
	}
	parked := p.process.Suspension()
	if parked == nil {
		return fmt.Errorf("engine: process %s has no suspension", p.process.ID())
	}
	response, err := suspension.EncodeResolution(resolution)
	if err != nil {
		return err
	}
	segment, err := p.owner.runtime.ResumeAsync(ctx, p.runCtx, p.process.ID(), parked.ID, response)
	if err != nil {
		return err
	}
	p.segment = segment
	return nil
}

func (p *turnProcess) Suspension() *agent.Suspension { return p.process.Suspension() }

func (p *turnProcess) Discard(ctx context.Context) error {
	if p == nil || p.process == nil || p.owner == nil || p.owner.runtime == nil {
		return errors.New("agentexec: discard process: incomplete turn process")
	}
	if !p.process.Status().IsTerminal() {
		return fmt.Errorf("agentexec: discard process %q: %w", p.process.ID(), runtime.ErrProcessActive)
	}
	if p.owner.processStore != nil {
		if err := p.owner.processStore.DeleteTrees(ctx, []string{p.process.ID()}); err != nil {
			return fmt.Errorf("agentexec: discard process snapshots: %w", err)
		}
	}
	return p.owner.runtime.RemoveTree(ctx, p.process.ID())
}

func (p *turnProcess) persistWaitingCheckpoint(status core.ProcessStatus) error {
	if p == nil || p.process == nil || p.owner == nil || p.owner.runtime == nil || p.owner.processStore == nil {
		return nil
	}
	if status != core.StatusWaiting {
		return nil
	}
	base := p.runCtx
	if base == nil {
		base = context.Background()
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(base), checkpointCommitTimeout)
	defer cancel()
	tree, err := p.owner.runtime.SnapshotTree(ctx, p.process.ID())
	if err != nil {
		return fmt.Errorf("agentexec: capture process tree: %w", err)
	}
	root, ok := processTreeRoot(tree)
	if !ok {
		return fmt.Errorf("agentexec: capture process tree: %w", core.ErrInvalidSnapshot)
	}
	if root.Status.IsTerminal() {
		// Cancellation may win after the segment reported Waiting but before
		// capture acquires the process tree. A terminal tree has no continuation
		// and must not replace the last usable checkpoint.
		return nil
	}
	if err := runtime.ValidateResumableSnapshot(root); err != nil {
		return fmt.Errorf("agentexec: validate waiting checkpoint: %w", err)
	}
	if p.usage == nil {
		return errors.New("agentexec: persist process tree: usage ledger is missing")
	}
	usage := p.usage.snapshot()
	if err := validateCheckpointUsage(tree, usage); err != nil {
		return fmt.Errorf("agentexec: validate checkpoint usage: %w", err)
	}
	checkpoint := execution.ProcessCheckpoint{
		BuildID:  p.owner.buildID,
		Scope:    p.scope,
		Provider: p.provider,
		Budget:   p.budget,
		Usage:    usage,
	}
	if err := p.owner.processStore.SaveTree(ctx, tree, checkpoint); err != nil {
		return fmt.Errorf("agentexec: persist process tree: %w", err)
	}
	return nil
}
