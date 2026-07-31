package agentexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/suspension"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
)

// TurnCompletion is the typed application projection of one Agent runtime
// segment completion.
//
// A segment reports its process failure and its own driving error separately,
// and how those two combine is the framework's rule — not something to restate
// here. Err is what that rule produced, with Runtime's own terminal work (the
// waiting checkpoint) appended.
type TurnCompletion struct {
	Status    core.ProcessStatus
	Output    TurnOutput
	HasOutput bool
	Err       error
}

// PendingSuspension is the executor-neutral projection of one unanswered
// Agent-runtime suspension. Framework checkpoint state never crosses this
// adapter boundary.
type PendingSuspension struct {
	ProcessID    string
	SuspensionID string
	Prompt       []byte
}

// SuspensionAnswer is one executor-owned suspension decision. Application item
// identity has already served its validation purpose at this boundary.
type SuspensionAnswer struct {
	ProcessID    string
	SuspensionID string
	Resolution   interrupts.Resolution
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

	// CancelSubtree terminates exactly one descendant process and everything
	// below it. The target must belong to this turn's process tree and cannot be
	// the root process itself.
	CancelSubtree(ctx context.Context, processID string) error

	// Resume atomically accepts the complete answer set exposed by the last
	// waiting boundary and starts its first continuation. Await automatically
	// drives any intermediate runtime-only waiting segments until every accepted
	// answer has been consumed or a new external boundary is reached.
	Resume(ctx context.Context, answers []SuspensionAnswer) error

	// PendingSuspensions returns the complete stable set of direct unanswered
	// boundaries in the process tree. Parent copies used only to propagate child
	// control flow are excluded.
	PendingSuspensions(ctx context.Context) ([]PendingSuspension, error)

	// Discard releases a TERMINATED process: it removes the process from the
	// engine registry and deletes any persisted waiting snapshot. Only a parked
	// process needs a restart checkpoint; terminal segments are never persisted.
	// A failure retains the terminal runtime tree so cleanup can retry the same
	// deletion. Calling Discard on a non-terminal process fails before storage
	// is touched.
	Discard(ctx context.Context) error
}

// WaitingSubtreePlanner is the optional mutation capability implemented by
// the real Agent runtime process. Keeping it separate from TurnProcess lets
// ordinary execution tests and alternate executors implement only the lifecycle
// surface they actually support.
type WaitingSubtreePlanner interface {
	PlanWaitingSubtreeCancellation(
		ctx context.Context,
		processID string,
	) (WaitingSubtreeCancellationPlan, error)
}

// WaitingSubtreeCancellationPlan is the Agent-execution adapter's private
// projection of a framework transition plan. It deliberately exposes no
// FrameworkState or storage representation to application/runs.
type WaitingSubtreeCancellationPlan interface {
	CanceledProcessIDs() []string
	PendingSuspensions() []PendingSuspension
	PersistCheckpoint(ctx context.Context) error
	Apply(ctx context.Context) error
	Continue(ctx context.Context) error
}

// turnProcess is the canonical [TurnProcess] backed by a real
// [runtime.Process]. It is package-private, so retaining the concrete Agent
// runtime keeps lifecycle commands inside this execution adapter.
type turnProcess struct {
	process          *runtime.Process
	segment          *runtime.Segment
	owner            *Engine
	scope            execution.TurnScope
	runCtx           context.Context
	usage            *usageLedger
	provider         string
	budget           accounting.Budget
	pendingResponses map[suspensionKey]json.RawMessage
}

const checkpointCommitTimeout = 10 * time.Second

type suspensionKey struct {
	processID    string
	suspensionID string
}

func (p *turnProcess) ID() string { return p.process.ID() }

func (p *turnProcess) Await() TurnCompletion {
	if p == nil || p.process == nil || p.segment == nil {
		return TurnCompletion{Err: errors.New("agentexec: await process: no active segment")}
	}
	for {
		segmentCompletion, err := p.segment.Await(context.Background())
		if err != nil {
			return TurnCompletion{Err: err}
		}
		p.segment = nil
		completion := TurnCompletion{
			Status: segmentCompletion.Status,
			Err:    segmentCompletion.Error(),
		}
		if output, ok := runtime.CompletionResult[TurnOutput](segmentCompletion); ok {
			completion.Output = output
			completion.HasOutput = true
		}
		completion.Err = errors.Join(completion.Err, p.persistWaitingCheckpoint(completion.Status))
		if completion.Err != nil || completion.Status != core.StatusWaiting || len(p.pendingResponses) == 0 {
			if completion.Status != core.StatusWaiting && len(p.pendingResponses) > 0 {
				completion.Err = errors.Join(
					completion.Err,
					fmt.Errorf(
						"agentexec: process terminated with %d accepted suspension responses unconsumed",
						len(p.pendingResponses),
					),
				)
				p.pendingResponses = nil
			}
			return completion
		}
		if err := p.resumeNext(context.Background()); err != nil {
			completion.Err = err
			p.pendingResponses = nil
			return completion
		}
	}
}

func (p *turnProcess) Cancel(ctx context.Context) error {
	if p == nil || p.owner == nil || p.owner.runtime == nil || p.process == nil {
		return errors.New("agentexec: cancel process: incomplete turn process")
	}
	return p.owner.runtime.Kill(ctx, p.process.ID())
}

func (p *turnProcess) CancelSubtree(ctx context.Context, processID string) error {
	if p == nil || p.owner == nil || p.owner.runtime == nil || p.process == nil {
		return errors.New("agentexec: cancel process subtree: incomplete turn process")
	}
	if processID == "" {
		return errors.New("agentexec: cancel process subtree: target process id is required")
	}
	if processID == p.process.ID() {
		return fmt.Errorf(
			"agentexec: cancel process subtree: target %q is the turn root",
			processID,
		)
	}
	target, found := p.owner.runtime.Process(processID)
	if !found {
		return fmt.Errorf(
			"agentexec: cancel process subtree: target process %q not found",
			processID,
		)
	}
	if err := p.requireDescendant(target); err != nil {
		return err
	}
	return p.owner.runtime.Kill(ctx, processID)
}

func (p *turnProcess) PlanWaitingSubtreeCancellation(
	ctx context.Context,
	processID string,
) (WaitingSubtreeCancellationPlan, error) {
	if p == nil || p.owner == nil || p.owner.runtime == nil || p.process == nil {
		return nil, errors.New("agentexec: plan waiting subtree cancellation: incomplete turn process")
	}
	if processID == "" {
		return nil, errors.New("agentexec: plan waiting subtree cancellation: target process id is required")
	}
	if processID == p.process.ID() {
		return nil, fmt.Errorf(
			"agentexec: plan waiting subtree cancellation: target %q is the turn root",
			processID,
		)
	}
	plan, err := p.owner.runtime.PlanWaitingSubtreeCancellation(
		ctx,
		p.process.ID(),
		processID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"agentexec: plan waiting subtree cancellation for process %q: %w",
			processID,
			err,
		)
	}
	return &waitingSubtreeCancellationPlan{process: p, runtime: plan}, nil
}

type waitingSubtreeCancellationPlan struct {
	process *turnProcess
	runtime *runtime.WaitingSubtreeCancellationPlan
}

func (plan *waitingSubtreeCancellationPlan) CanceledProcessIDs() []string {
	if plan == nil || plan.runtime == nil {
		return nil
	}
	return plan.runtime.CanceledProcessIDs()
}

func (plan *waitingSubtreeCancellationPlan) PendingSuspensions() []PendingSuspension {
	if plan == nil || plan.runtime == nil {
		return nil
	}
	pending := plan.runtime.PendingSuspensions()
	out := make([]PendingSuspension, len(pending))
	for index, boundary := range pending {
		out[index] = PendingSuspension{
			ProcessID:    boundary.ProcessID,
			SuspensionID: boundary.SuspensionID,
			Prompt:       append([]byte(nil), boundary.Prompt...),
		}
	}
	return out
}

func (plan *waitingSubtreeCancellationPlan) PersistCheckpoint(ctx context.Context) error {
	if plan == nil || plan.runtime == nil || plan.process == nil {
		return errors.New("agentexec: persist waiting subtree cancellation plan: incomplete plan")
	}
	return plan.process.persistProcessTree(ctx, plan.runtime.ResultingTree())
}

func (plan *waitingSubtreeCancellationPlan) Apply(ctx context.Context) error {
	if plan == nil || plan.runtime == nil || plan.process == nil ||
		plan.process.owner == nil || plan.process.owner.runtime == nil {
		return errors.New("agentexec: apply waiting subtree cancellation plan: incomplete plan")
	}
	return plan.process.owner.runtime.ApplyWaitingSubtreeCancellation(ctx, plan.runtime)
}

func (plan *waitingSubtreeCancellationPlan) Continue(ctx context.Context) error {
	if plan == nil || plan.process == nil {
		return errors.New("agentexec: continue waiting subtree cancellation: incomplete plan")
	}
	process := plan.process
	if process.segment != nil {
		return fmt.Errorf(
			"agentexec: continue waiting subtree cancellation: process %q already has an active segment",
			process.process.ID(),
		)
	}
	segment, err := process.owner.runtime.ContinueAsync(ctx, process.process.ID())
	if err != nil {
		return fmt.Errorf(
			"agentexec: continue waiting subtree cancellation for process %q: %w",
			process.process.ID(),
			err,
		)
	}
	process.segment = segment
	return nil
}

func (p *turnProcess) requireDescendant(target *runtime.Process) error {
	rootID := p.process.ID()
	seen := make(map[string]struct{})
	for current := target; current != nil; {
		parentID := current.ParentID()
		if parentID == rootID {
			return nil
		}
		if parentID == "" {
			break
		}
		if _, duplicate := seen[parentID]; duplicate {
			return fmt.Errorf(
				"agentexec: cancel process subtree: process ancestry from %q contains a cycle at %q",
				target.ID(),
				parentID,
			)
		}
		seen[parentID] = struct{}{}
		parent, found := p.owner.runtime.Process(parentID)
		if !found {
			return fmt.Errorf(
				"agentexec: cancel process subtree: process %q has missing ancestor %q",
				target.ID(),
				parentID,
			)
		}
		current = parent
	}
	return fmt.Errorf(
		"agentexec: cancel process subtree: process %q does not belong to turn root %q",
		target.ID(),
		rootID,
	)
}

func (p *turnProcess) Resume(ctx context.Context, answers []SuspensionAnswer) error {
	if p == nil || p.process == nil || p.owner == nil || p.owner.runtime == nil {
		return errors.New("agentexec: resume process: incomplete turn process")
	}
	if p.segment != nil {
		return fmt.Errorf("agentexec: resume process %q: segment is already active", p.process.ID())
	}
	pending, err := p.PendingSuspensions(ctx)
	if err != nil {
		return err
	}
	if len(answers) != len(pending) {
		return fmt.Errorf(
			"agentexec: resume process %q: %d answers do not cover %d pending suspensions",
			p.process.ID(),
			len(answers),
			len(pending),
		)
	}
	responses := make(map[suspensionKey]json.RawMessage, len(answers))
	for _, answer := range answers {
		key := suspensionKey{processID: answer.ProcessID, suspensionID: answer.SuspensionID}
		if key.processID == "" || key.suspensionID == "" {
			return fmt.Errorf("agentexec: resume process %q: answer has incomplete suspension identity", p.process.ID())
		}
		if _, duplicate := responses[key]; duplicate {
			return fmt.Errorf(
				"agentexec: resume process %q: duplicate answer for process %q suspension %q",
				p.process.ID(),
				key.processID,
				key.suspensionID,
			)
		}
		response, err := suspension.EncodeResolution(answer.Resolution)
		if err != nil {
			return fmt.Errorf(
				"agentexec: resume process %q: encode answer for process %q suspension %q: %w",
				p.process.ID(),
				key.processID,
				key.suspensionID,
				err,
			)
		}
		responses[key] = response
	}
	for _, boundary := range pending {
		key := suspensionKey{processID: boundary.ProcessID, suspensionID: boundary.SuspensionID}
		if _, ok := responses[key]; !ok {
			return fmt.Errorf(
				"agentexec: resume process %q: no answer for process %q suspension %q",
				p.process.ID(),
				key.processID,
				key.suspensionID,
			)
		}
	}
	p.pendingResponses = responses
	if err := p.resumeNext(ctx); err != nil {
		p.pendingResponses = nil
		return err
	}
	return nil
}

func (p *turnProcess) resumeNext(ctx context.Context) error {
	// A waiting-tree transformation may have made one framework-owned boundary
	// internally ready ahead of the external answer set. Advance it first,
	// without fabricating a Resume event or consuming a human response.
	segment, continueErr := p.owner.runtime.ContinueAsync(ctx, p.process.ID())
	if continueErr == nil {
		p.segment = segment
		return nil
	}
	if !errors.Is(continueErr, interaction.ErrSuspensionStale) {
		return continueErr
	}

	pending, err := p.PendingSuspensions(ctx)
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		return fmt.Errorf("agentexec: resume process %q: waiting tree has no pending suspension", p.process.ID())
	}
	next := pending[0]
	key := suspensionKey{processID: next.ProcessID, suspensionID: next.SuspensionID}
	response, ok := p.pendingResponses[key]
	if !ok {
		return fmt.Errorf(
			"agentexec: resume process %q: newly exposed process %q suspension %q was not in the accepted answer set",
			p.process.ID(),
			next.ProcessID,
			next.SuspensionID,
		)
	}
	parked := p.process.Suspension()
	if parked == nil {
		return fmt.Errorf("agentexec: resume process %q: root has no promoted suspension", p.process.ID())
	}
	segment, err = p.owner.runtime.ResumeAsync(ctx, p.runCtx, p.process.ID(), parked.ID, response)
	if err != nil {
		return err
	}
	delete(p.pendingResponses, key)
	p.segment = segment
	return nil
}

func (p *turnProcess) PendingSuspensions(ctx context.Context) ([]PendingSuspension, error) {
	if p == nil || p.process == nil || p.owner == nil || p.owner.runtime == nil {
		return nil, errors.New("agentexec: inspect pending suspensions: incomplete turn process")
	}
	pending, err := p.owner.runtime.PendingSuspensions(ctx, p.process.ID())
	if err != nil {
		return nil, fmt.Errorf("agentexec: inspect pending suspensions: %w", err)
	}
	out := make([]PendingSuspension, len(pending))
	for index, suspension := range pending {
		out[index] = PendingSuspension{
			ProcessID:    suspension.ProcessID,
			SuspensionID: suspension.SuspensionID,
			Prompt:       append([]byte(nil), suspension.Prompt...),
		}
	}
	return out, nil
}

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
	root, ok := tree.Root()
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
	return p.persistProcessTree(ctx, tree)
}

func (p *turnProcess) persistProcessTree(
	ctx context.Context,
	tree core.ProcessSnapshotTree,
) error {
	if p == nil || p.process == nil || p.owner == nil || p.owner.processStore == nil {
		return errors.New("agentexec: persist process tree: incomplete turn process")
	}
	if p.usage == nil {
		return errors.New("agentexec: persist process tree: usage ledger is missing")
	}
	usage, err := p.usage.snapshot()
	if err != nil {
		return fmt.Errorf("agentexec: capture usage projection: %w", err)
	}
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
	state, err := encodeProcessTreeState(tree)
	if err != nil {
		return fmt.Errorf("agentexec: persist process tree: %w", err)
	}
	if err := p.owner.processStore.SaveTree(ctx, state, checkpoint); err != nil {
		return fmt.Errorf("agentexec: persist process tree: %w", err)
	}
	return nil
}
