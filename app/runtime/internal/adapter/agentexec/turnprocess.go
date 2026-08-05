package agentexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/suspension"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

// TurnCompletion is the typed application projection of one Agent runtime run
// completion. Durable checkpoint policy is deliberately absent: a
// waiting checkpoint is captured only after the application accepts the
// boundary, then committed by its tree-barrier transaction.
//
// A run reports its process failure and its own driving error separately,
// and how those two combine is the framework's rule — not something to restate
// here. Err is exactly what that rule produced.
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
// The interface is the Engine's process result and keeps its runtime process
// implementation private.
type TurnProcess interface {
	// ID is the private root process identity used to route executor control.
	ID() string

	// Await joins the active run and captures its immutable completion.
	// Exactly one owner calls Await for each initial or resumed run.
	Await() TurnCompletion

	// Cancel marks the process [core.StatusKilled] via the engine. The ongoing
	// tick observes the status flip at its next checkpoint and completes its
	// active run.
	Cancel(ctx context.Context) error

	// CancelSubtree terminates exactly one descendant process and everything
	// below it. The target must belong to this turn's process tree and cannot be
	// the root process itself.
	CancelSubtree(ctx context.Context, processID string) error

	// Resume atomically accepts the complete answer set exposed by the last
	// waiting boundary and starts its first continuation. Await automatically
	// drives any intermediate runtime-only waiting runs until every accepted
	// answer has been consumed or a new external boundary is reached.
	Resume(ctx context.Context, answers []SuspensionAnswer) error

	// PendingSuspensions returns the complete stable set of direct unanswered
	// boundaries in the process tree. Parent copies used only to propagate child
	// control flow are excluded.
	PendingSuspensions(ctx context.Context) ([]PendingSuspension, error)

	// CaptureWaitingCheckpoint captures one immutable waiting tree and its
	// unanswered boundaries without performing I/O.
	CaptureWaitingCheckpoint(ctx context.Context) (WaitingCheckpoint, error)

	// Discard releases a TERMINATED process from the live framework registry.
	// Durable checkpoint deletion belongs to the Application terminal write-set,
	// so this method performs no storage I/O. A failure retains the terminal tree
	// so cleanup can retry. Calling Discard on an active process is rejected.
	Discard(ctx context.Context) error
}

// WaitingCheckpoint is the data-only application projection of one Agent
// waiting tree. Checkpoint contains one root-owned opaque executor aggregate;
// Suspensions are the external boundaries captured from that exact immutable
// tree.
type WaitingCheckpoint struct {
	Checkpoint  execution.ExecutorCheckpoint
	Suspensions []PendingSuspension
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
	Checkpoint() execution.ExecutorCheckpoint
	Apply(ctx context.Context) error
	Continue(ctx context.Context) error
}

// turnProcess is the canonical [TurnProcess] backed by a real
// [runtime.Process]. It is package-private, so retaining the concrete Agent
// runtime keeps lifecycle commands inside this execution adapter.
type turnProcess struct {
	process          *runtime.Process
	runHandle        *runtime.RunHandle
	owner            *Engine
	scope            execution.ExecutionScope
	runCtx           context.Context
	usage            *usageLedger
	modelSelection   modelref.Selection
	limits           execution.RunLimits
	pendingResponses map[suspensionKey]json.RawMessage
}

type suspensionKey struct {
	processID    string
	suspensionID string
}

func (p *turnProcess) ID() string { return p.process.ID() }

func (p *turnProcess) Await() TurnCompletion {
	if p == nil || p.process == nil || p.runHandle == nil {
		return TurnCompletion{Err: errors.New("agentexec: await process: no active run")}
	}
	ctx := p.detachedRunContext()
	for {
		runCompletion, err := p.runHandle.Await(ctx)
		if err != nil {
			return TurnCompletion{Err: err}
		}
		p.runHandle = nil
		completion := TurnCompletion{
			Status: runCompletion.Status,
			Err:    runCompletion.Error(),
		}
		if output, ok := runtime.CompletionResult[TurnOutput](runCompletion); ok {
			completion.Output = output
			completion.HasOutput = true
		}
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
		if err := p.resumeNext(ctx); err != nil {
			completion.Err = err
			p.pendingResponses = nil
			return completion
		}
	}
}

// detachedRunContext preserves the immutable execution scope, model selection,
// and trace lineage while letting the process owner join and auto-continue a
// run independently of the request that initiated it.
func (p *turnProcess) detachedRunContext() context.Context {
	if p == nil || p.runCtx == nil {
		return context.Background()
	}
	return context.WithoutCancel(p.runCtx)
}

func (p *turnProcess) Cancel(ctx context.Context) error {
	if p == nil || p.owner == nil || p.owner.agentRuntime == nil || p.process == nil {
		return errors.New("agentexec: cancel process: incomplete turn process")
	}
	return p.owner.agentRuntime.Kill(ctx, p.process.ID())
}

func (p *turnProcess) CancelSubtree(ctx context.Context, processID string) error {
	if p == nil || p.owner == nil || p.owner.agentRuntime == nil || p.process == nil {
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
	target, found := p.owner.agentRuntime.Process(processID)
	if !found {
		return fmt.Errorf(
			"agentexec: cancel process subtree: target process %q not found",
			processID,
		)
	}
	if err := p.requireDescendant(target); err != nil {
		return err
	}
	return p.owner.agentRuntime.Kill(ctx, processID)
}

func (p *turnProcess) PlanWaitingSubtreeCancellation(
	ctx context.Context,
	processID string,
) (WaitingSubtreeCancellationPlan, error) {
	if p == nil || p.owner == nil || p.owner.agentRuntime == nil || p.process == nil {
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
	plan, err := p.owner.agentRuntime.PlanWaitingSubtreeCancellation(
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
	checkpoint, err := p.captureProcessTree(plan.ResultingTree())
	if err != nil {
		return nil, fmt.Errorf(
			"agentexec: capture waiting subtree cancellation checkpoint for process %q: %w",
			processID,
			err,
		)
	}
	return &waitingSubtreeCancellationPlan{process: p, runtimePlan: plan, checkpoint: checkpoint}, nil
}

type waitingSubtreeCancellationPlan struct {
	process     *turnProcess
	runtimePlan *runtime.WaitingSubtreeCancellationPlan
	checkpoint  execution.ExecutorCheckpoint
}

func (plan *waitingSubtreeCancellationPlan) CanceledProcessIDs() []string {
	if plan == nil || plan.runtimePlan == nil {
		return nil
	}
	return plan.runtimePlan.CanceledProcessIDs()
}

func (plan *waitingSubtreeCancellationPlan) PendingSuspensions() []PendingSuspension {
	if plan == nil || plan.runtimePlan == nil {
		return nil
	}
	pending := plan.runtimePlan.PendingSuspensions()
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

func (plan *waitingSubtreeCancellationPlan) Checkpoint() execution.ExecutorCheckpoint {
	if plan == nil {
		return execution.ExecutorCheckpoint{}
	}
	return plan.checkpoint.Clone()
}

func (plan *waitingSubtreeCancellationPlan) Apply(ctx context.Context) error {
	if plan == nil || plan.runtimePlan == nil || plan.process == nil ||
		plan.process.owner == nil || plan.process.owner.agentRuntime == nil {
		return errors.New("agentexec: apply waiting subtree cancellation plan: incomplete plan")
	}
	return plan.process.owner.agentRuntime.ApplyWaitingSubtreeCancellation(ctx, plan.runtimePlan)
}

func (plan *waitingSubtreeCancellationPlan) Continue(ctx context.Context) error {
	if plan == nil || plan.process == nil {
		return errors.New("agentexec: continue waiting subtree cancellation: incomplete plan")
	}
	process := plan.process
	if process.runHandle != nil {
		return fmt.Errorf(
			"agentexec: continue waiting subtree cancellation: process %q already has an active run",
			process.process.ID(),
		)
	}
	runHandle, err := process.owner.agentRuntime.ContinueAsync(ctx, process.process.ID())
	if err != nil {
		return fmt.Errorf(
			"agentexec: continue waiting subtree cancellation for process %q: %w",
			process.process.ID(),
			err,
		)
	}
	process.runHandle = runHandle
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
		parent, found := p.owner.agentRuntime.Process(parentID)
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
	if p == nil || p.process == nil || p.owner == nil || p.owner.agentRuntime == nil {
		return errors.New("agentexec: resume process: incomplete turn process")
	}
	if p.runHandle != nil {
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
	runHandle, continueErr := p.owner.agentRuntime.ContinueAsync(ctx, p.process.ID())
	if continueErr == nil {
		p.runHandle = runHandle
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
	runHandle, err = p.owner.agentRuntime.RespondAndContinueAsync(ctx, p.runCtx, p.process.ID(), parked.ID, response)
	if err != nil {
		return err
	}
	delete(p.pendingResponses, key)
	p.runHandle = runHandle
	return nil
}

func (p *turnProcess) PendingSuspensions(ctx context.Context) ([]PendingSuspension, error) {
	if p == nil || p.process == nil || p.owner == nil || p.owner.agentRuntime == nil {
		return nil, errors.New("agentexec: inspect pending suspensions: incomplete turn process")
	}
	pending, err := p.owner.agentRuntime.PendingSuspensions(ctx, p.process.ID())
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

func (p *turnProcess) CaptureWaitingCheckpoint(ctx context.Context) (WaitingCheckpoint, error) {
	if p == nil || p.process == nil || p.owner == nil || p.owner.agentRuntime == nil {
		return WaitingCheckpoint{}, errors.New("agentexec: capture waiting checkpoint: incomplete turn process")
	}
	tree, err := p.owner.agentRuntime.SnapshotTree(ctx, p.process.ID())
	if err != nil {
		return WaitingCheckpoint{}, fmt.Errorf("agentexec: capture waiting checkpoint: %w", err)
	}
	root, ok := tree.Root()
	if !ok {
		return WaitingCheckpoint{}, fmt.Errorf("agentexec: capture waiting checkpoint: %w", core.ErrInvalidSnapshot)
	}
	if err := runtime.ValidateResumableSnapshot(root); err != nil {
		return WaitingCheckpoint{}, fmt.Errorf("agentexec: capture waiting checkpoint: %w", err)
	}
	pending, err := runtime.PendingSuspensionsIn(tree)
	if err != nil {
		return WaitingCheckpoint{}, fmt.Errorf("agentexec: capture waiting checkpoint boundaries: %w", err)
	}
	if len(pending) == 0 {
		return WaitingCheckpoint{}, errors.New("agentexec: capture waiting checkpoint: tree has no unanswered suspension")
	}
	captured, err := p.captureProcessTree(tree)
	if err != nil {
		return WaitingCheckpoint{}, err
	}
	suspensions := make([]PendingSuspension, len(pending))
	for index, boundary := range pending {
		suspensions[index] = PendingSuspension{
			ProcessID:    boundary.ProcessID,
			SuspensionID: boundary.SuspensionID,
			Prompt:       append([]byte(nil), boundary.Prompt...),
		}
	}
	return WaitingCheckpoint{Checkpoint: captured, Suspensions: suspensions}, nil
}

func (p *turnProcess) Discard(ctx context.Context) error {
	if p == nil || p.process == nil || p.owner == nil || p.owner.agentRuntime == nil {
		return errors.New("agentexec: discard process: incomplete turn process")
	}
	if !p.process.Status().IsTerminal() {
		return fmt.Errorf("agentexec: discard process %q: %w", p.process.ID(), runtime.ErrProcessActive)
	}
	return p.owner.agentRuntime.RemoveTree(ctx, p.process.ID())
}

func (p *turnProcess) captureProcessTree(
	tree core.ProcessSnapshotTree,
) (execution.ExecutorCheckpoint, error) {
	if p == nil || p.process == nil || p.owner == nil {
		return execution.ExecutorCheckpoint{}, errors.New("agentexec: capture process tree: incomplete turn process")
	}
	if p.usage == nil {
		return execution.ExecutorCheckpoint{}, errors.New("agentexec: capture process tree: usage ledger is missing")
	}
	usage, err := p.usage.snapshot()
	if err != nil {
		return execution.ExecutorCheckpoint{}, fmt.Errorf("agentexec: capture process tree usage: %w", err)
	}
	if err := validateCheckpointUsage(tree, usage); err != nil {
		return execution.ExecutorCheckpoint{}, fmt.Errorf("agentexec: capture process tree usage: %w", err)
	}
	payload, err := encodeProcessTree(tree)
	if err != nil {
		return execution.ExecutorCheckpoint{}, fmt.Errorf("agentexec: capture process tree: %w", err)
	}
	checkpoint := execution.ExecutorCheckpoint{
		RootProcessID:  tree.RootID,
		Payload:        payload,
		BuildID:        p.owner.buildID,
		Scope:          p.scope,
		ModelSelection: p.modelSelection,
		Limits:         p.limits,
		Usage:          usage,
	}
	if err := checkpoint.Validate(); err != nil {
		return execution.ExecutorCheckpoint{}, fmt.Errorf("agentexec: capture process tree: %w", err)
	}
	return checkpoint, nil
}
