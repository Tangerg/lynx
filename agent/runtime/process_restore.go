package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/planning"
)

// ErrResumableSnapshotLost reports that a stored process no longer contains a
// compatible waiting continuation for this Engine.
var ErrResumableSnapshotLost = errors.New("resumable process snapshot lost")

type continuationStateError struct {
	err error
}

func (e *continuationStateError) Error() string { return e.err.Error() }
func (e *continuationStateError) Unwrap() error { return e.err }

func continuationStateErrorf(format string, args ...any) error {
	return &continuationStateError{err: fmt.Errorf(format, args...)}
}

// continuationLoss marks permanent snapshot/continuation incompatibility while
// preserving the concrete cause for errors.Is/errors.As. Operational store and
// coordination failures deliberately remain outside this lane.
func continuationLoss(err error) error {
	if err == nil {
		return nil
	}
	var loss *continuationStateError
	if errors.As(err, &loss) {
		return err
	}
	return &continuationStateError{err: err}
}

func resumableSnapshotLost(operation string, err error) error {
	return fmt.Errorf("%s: %w: %w", operation, ErrResumableSnapshotLost, err)
}

// Restore loads a snapshot from the configured store and
// rebuilds an [Process] bound to a currently-deployed agent
// definition. The restored process is registered in the engine's
// process map and ready for inspection or (when the snapshot status
// is resumable) re-entry into the tick loop via the standard run
// surface.
//
// Errors propagate from the store and from agent re-binding (the
// agent must be deployed under the same name as recorded in the
// snapshot).
//
// options re-attaches the per-process wiring (Extensions + Session) the
// continuation needs — see [Engine.RestoreSnapshot]. Pass the zero
// value for a read-only restore.
func (e *Engine) Restore(ctx context.Context, processID string, options core.ProcessOptions) (*Process, error) {
	if e == nil {
		return nil, errors.New("runtime.Engine.Restore: nil Engine")
	}
	if e.processStore == nil {
		return nil, errors.New("runtime.Engine.Restore: no ProcessStore configured")
	}
	ctx = normalizeContext(ctx)
	treeKey := e.processTreeKey(processID, "")
	releaseWrite, err := e.processWrites.acquire(ctx, treeKey)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.Restore: order durable read: %w", err)
	}
	snapshot, err := e.processStore.Load(ctx, processID)
	releaseWrite()
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.Restore: %w", err)
	}
	return e.restoreSnapshot(ctx, "runtime.Engine.Restore", snapshot, options)
}

// ValidateResumableSnapshot verifies that a durable process snapshot contains
// a continuation the runtime can safely resume. It is intentionally independent
// of any Host persistence schema: stores may call it after decoding a snapshot
// to decide whether a parked application record still has a usable framework
// continuation.
//
// Human suspensions carry their continuation entirely in the typed action's
// durable blackboard state. Tool suspensions additionally carry a managed
// ToolLoop checkpoint envelope; this function validates that opaque runtime
// payload and its exact deployment/ID binding so Hosts never need to interpret
// framework checkpoint fields themselves.
func ValidateResumableSnapshot(snapshot core.ProcessSnapshot) error {
	if err := snapshot.Validate(); err != nil {
		return fmt.Errorf("runtime.ValidateResumableSnapshot: %w", err)
	}
	if snapshot.Status != core.StatusWaiting || snapshot.Suspension == nil {
		return fmt.Errorf("runtime.ValidateResumableSnapshot: process %q is not waiting on a suspension", snapshot.ID)
	}
	suspension := snapshot.Suspension
	if suspension.Responded() {
		return fmt.Errorf("runtime.ValidateResumableSnapshot: suspension %q already has a response", suspension.ID)
	}
	checkpoint, recognized, err := decodeSuspensionCheckpoint(suspension.Payload)
	if err != nil {
		return fmt.Errorf("runtime.ValidateResumableSnapshot: %w", err)
	}
	if !recognized {
		if suspension.Kind == interaction.SuspensionTool {
			return errors.New("runtime.ValidateResumableSnapshot: tool suspension has no managed checkpoint")
		}
		return nil
	}
	if checkpoint.Kind == suspensionCheckpointNestedChild {
		return nil
	}
	if checkpoint.Deployment != snapshot.Deployment {
		return errors.New("runtime.ValidateResumableSnapshot: tool checkpoint deployment does not match snapshot deployment")
	}
	if checkpoint.Checkpoint.ID != suspension.ID {
		return fmt.Errorf("runtime.ValidateResumableSnapshot: tool checkpoint ID %q does not match suspension ID %q", checkpoint.Checkpoint.ID, suspension.ID)
	}
	return nil
}

// Resumable reports whether processID names a structurally valid waiting
// snapshot whose exact deployment is owned by this Engine. Missing, corrupt,
// non-waiting, and deployment-incompatible snapshots return false, nil;
// persistence access failures are returned as errors.
func (e *Engine) Resumable(ctx context.Context, processID string) (bool, error) {
	if e == nil {
		return false, errors.New("runtime.Engine.Resumable: nil Engine")
	}
	ctx = normalizeContext(ctx)
	releaseWrite, err := e.processWrites.acquire(ctx, e.processTreeKey(processID, ""))
	if err != nil {
		return false, fmt.Errorf("runtime.Engine.Resumable: order durable read: %w", err)
	}
	defer releaseWrite()
	_, err = e.loadResumableTree(ctx, processID, map[string]struct{}{}, true)
	if err == nil {
		return true, nil
	}
	var loss *continuationStateError
	if errors.As(err, &loss) {
		return false, nil
	}
	return false, fmt.Errorf("runtime.Engine.Resumable: %w", err)
}

// RestoreResumable loads and rebuilds a waiting continuation. Every durable
// state loss or incompatibility wraps ErrResumableSnapshotLost; persistence
// access failures remain ordinary errors so hosts can distinguish an unusable
// continuation from a store access failure.
func (e *Engine) RestoreResumable(ctx context.Context, processID string, options core.ProcessOptions) (*Process, error) {
	if e == nil {
		return nil, errors.New("runtime.Engine.RestoreResumable: nil Engine")
	}
	ctx = normalizeContext(ctx)
	treeKey := e.processTreeKey(processID, "")
	releaseWrite, err := e.processWrites.acquire(ctx, treeKey)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.RestoreResumable: order durable read: %w", err)
	}
	tree, err := e.loadResumableTree(ctx, processID, map[string]struct{}{}, true)
	releaseWrite()
	if err != nil {
		var loss *continuationStateError
		if errors.As(err, &loss) {
			return nil, resumableSnapshotLost("runtime.Engine.RestoreResumable", err)
		}
		return nil, fmt.Errorf("runtime.Engine.RestoreResumable: %w", err)
	}
	var processes []*Process
	process, err := e.restoreResumableTree(ctx, tree, options, nil, &processes)
	if err != nil {
		var stateErr *continuationStateError
		if errors.As(err, &stateErr) {
			return nil, resumableSnapshotLost(
				"runtime.Engine.RestoreResumable",
				fmt.Errorf("rebuild process %q: %w", processID, err),
			)
		}
		return nil, fmt.Errorf("runtime.Engine.RestoreResumable: rebuild process %q: %w", processID, err)
	}
	releaseMutation, err := e.processMutations.acquire(ctx, treeKey)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.RestoreResumable: acquire process tree: %w", err)
	}
	defer releaseMutation()
	if !e.processes.registerTree(processes) {
		return nil, fmt.Errorf("runtime.Engine.RestoreResumable: process tree %q is already registered", processID)
	}
	return process, nil
}

type resumableProcessTree struct {
	snapshot core.ProcessSnapshot
	children []*resumableProcessTree
}

// loadResumableTree classifies permanent continuation loss with
// continuationStateError while leaving operational store failures ordinary.
func (e *Engine) loadResumableTree(
	ctx context.Context,
	processID string,
	visited map[string]struct{},
	root bool,
) (*resumableProcessTree, error) {
	if _, duplicate := visited[processID]; duplicate {
		return nil, continuationLoss(fmt.Errorf("%w: nested process cycle at %q", core.ErrInvalidSnapshot, processID))
	}
	visited[processID] = struct{}{}
	snapshot, err := e.loadStoredSnapshot(ctx, processID)
	if err != nil {
		return nil, err
	}
	if root {
		if err := ValidateResumableSnapshot(snapshot); err != nil {
			return nil, continuationLoss(err)
		}
	} else if snapshot.Status != core.StatusWaiting && !snapshot.Status.IsTerminal() {
		return nil, continuationLoss(fmt.Errorf("%w: nested child %q has non-resumable status %s", core.ErrInvalidSnapshot, processID, snapshot.Status))
	}
	if _, ok := e.catalog.lookup(snapshot.Deployment); !ok {
		return nil, continuationLoss(fmt.Errorf("%w: %s", ErrDeploymentNotFound, snapshot.Deployment))
	}

	tree := &resumableProcessTree{snapshot: snapshot}
	checkpoint, relationErr := nestedChildrenFromSuspension(snapshot.Suspension)
	if relationErr != nil {
		return nil, continuationLoss(relationErr)
	}
	tree.children = make([]*resumableProcessTree, 0, len(checkpoint.relations))
	for _, relation := range checkpoint.relations {
		childTree, err := e.loadResumableTree(ctx, relation.ChildID, visited, false)
		if err != nil {
			return nil, err
		}
		if err := relation.validateSnapshot(snapshot, childTree.snapshot); err != nil {
			return nil, continuationLoss(err)
		}
		tree.children = append(tree.children, childTree)
	}
	return tree, nil
}

func (e *Engine) loadStoredSnapshot(ctx context.Context, processID string) (core.ProcessSnapshot, error) {
	if e == nil {
		return core.ProcessSnapshot{}, errors.New("nil engine")
	}
	if e.processStore == nil {
		return core.ProcessSnapshot{}, errors.New("no ProcessStore configured")
	}
	snapshot, err := e.processStore.Load(ctx, processID)
	if err != nil {
		if errors.Is(err, core.ErrSnapshotNotFound) ||
			errors.Is(err, core.ErrSnapshotSchema) ||
			errors.Is(err, core.ErrInvalidSnapshot) {
			return core.ProcessSnapshot{}, continuationLoss(err)
		}
		return core.ProcessSnapshot{}, fmt.Errorf("load process %q: %w", processID, err)
	}
	if snapshot.ID != processID {
		return core.ProcessSnapshot{}, continuationLoss(fmt.Errorf("%w: stored snapshot identity does not match process %q", core.ErrInvalidSnapshot, processID))
	}
	if err := snapshot.Validate(); err != nil {
		return core.ProcessSnapshot{}, continuationLoss(err)
	}
	return snapshot, nil
}

func (e *Engine) restoreResumableTree(
	ctx context.Context,
	tree *resumableProcessTree,
	options core.ProcessOptions,
	parent *Process,
	processes *[]*Process,
) (*Process, error) {
	if tree == nil {
		return nil, errors.New("runtime: resumable process tree is nil")
	}
	snapshot := tree.snapshot
	process, err := e.buildProcessSnapshot(snapshot, options)
	if err != nil {
		return nil, err
	}
	*processes = append(*processes, process)
	if parent != nil {
		linker := childRun{ctx: ctx, engine: e}
		if err := linker.restoreSession(process, parent); err != nil {
			return nil, fmt.Errorf("restore child session: %w", err)
		}
		parent.budget.addChild(process)
	}
	for _, childTree := range tree.children {
		childOptions, err := restoredChildOptions(ctx, process, e, childTree.snapshot.Deployment)
		if err != nil {
			return nil, err
		}
		if _, err := e.restoreResumableTree(ctx, childTree, childOptions, process, processes); err != nil {
			return nil, err
		}
	}
	return process, nil
}

func restoredChildOptions(
	ctx context.Context,
	parent *Process,
	engine *Engine,
	deploymentRef core.DeploymentRef,
) (core.ProcessOptions, error) {
	deployment, ok := engine.catalog.lookup(deploymentRef)
	if !ok {
		return core.ProcessOptions{}, fmt.Errorf("%w: %s", ErrDeploymentNotFound, deploymentRef)
	}
	options, err := configureChildProcessOptions(ctx, parent, deployment, core.ProcessOptions{})
	if err != nil {
		return core.ProcessOptions{}, err
	}
	extensions, err := parent.childExtensions(options.Extensions)
	if err != nil {
		return core.ProcessOptions{}, fmt.Errorf("restore child extensions: %w", err)
	}
	options.Extensions = extensions
	return options, nil
}

// RestoreSnapshot rebuilds a [Process] from a snapshot the
// caller already holds — the pure-rebuild primitive, no store I/O.
// ([Engine.Restore] is the store-backed sibling: it loads the
// snapshot by id, then calls this.) The process is added to engine's
// registry under the snapshot's id; the agent definition is looked up by the
// exact [core.ProcessSnapshot.Deployment] and must exist in the deployment
// catalog. Historical definitions remain eligible after replacement or
// undeploy.
//
// Resumable statuses (Running / Waiting / Paused) leave the process
// ready for re-entry into the tick loop. Terminal statuses
// (Completed / Failed / Killed / Terminated) load the process
// read-only; callers can inspect History / Usage / Failure but
// not re-run.
//
// A restored StatusWaiting process carries its exact Suspension and can be
// answered immediately. Resume records the response; Continue
// then re-enters the action at its linear suspension point.
//
// options carries the per-process wiring the snapshot can't hold — the
// session-scoped [core.ProcessOptions.Extensions] (observer / event
// listener / tool middleware) and the [core.ProcessOptions.Session]
// binding. A restored process re-enters the tick loop with the same
// observability + session context a fresh one gets from
// [Engine.Start], so the continuation streams and keys chat history
// correctly. Pass the zero value to restore read-only (audit / inspect).
func (e *Engine) RestoreSnapshot(
	ctx context.Context,
	snapshot core.ProcessSnapshot,
	options core.ProcessOptions,
) (*Process, error) {
	if e == nil {
		return nil, errors.New("runtime.Engine.RestoreSnapshot: nil Engine")
	}
	return e.restoreSnapshot(normalizeContext(ctx), "runtime.Engine.RestoreSnapshot", snapshot, options)
}

func (e *Engine) restoreSnapshot(
	ctx context.Context,
	operation string,
	snapshot core.ProcessSnapshot,
	options core.ProcessOptions,
) (*Process, error) {
	process, err := e.buildProcessSnapshot(snapshot, options)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	releaseMutation, err := e.processMutations.acquire(
		ctx,
		e.processTreeKey(snapshot.ID, snapshot.ParentID),
	)
	if err != nil {
		return nil, fmt.Errorf("%s: acquire process tree: %w", operation, err)
	}
	defer releaseMutation()
	if !e.processes.registerNew(process) {
		return nil, fmt.Errorf("%s: process %q is already registered", operation, process.id)
	}
	return process, nil
}

func (e *Engine) buildProcessSnapshot(snapshot core.ProcessSnapshot, options core.ProcessOptions) (*Process, error) {
	if e == nil {
		return nil, errors.New("nil engine")
	}
	if err := snapshot.Validate(); err != nil {
		return nil, &continuationStateError{err: err}
	}

	deployment, ok := e.catalog.lookup(snapshot.Deployment)
	if !ok {
		return nil, &continuationStateError{
			err: fmt.Errorf("%w: %s", ErrDeploymentNotFound, snapshot.Deployment),
		}
	}
	agent := deployment.agent

	processOptions, err := snapshotProcessOptions(options)
	if err != nil {
		return nil, err
	}
	dependencies, err := e.prepareProcessDependencies(options.Dependencies)
	if err != nil {
		return nil, err
	}
	blackboard, err := e.resolveBlackboard(options.Blackboard)
	if err != nil {
		return nil, err
	}
	planner, err := e.resolvePlanner(agent, processOptions.extensions)
	if err != nil {
		return nil, err
	}
	domain, err := planning.DomainForAgent(agent)
	if err != nil {
		return nil, fmt.Errorf("domain: %w", err)
	}

	process := newProcess(snapshot.ID, deployment, &processOptions, blackboard, dependencies, planner, domain, e)
	// Wire the state reader + event multicast the same way createProcess
	// does — without it a resumable snapshot panics on its first
	// post-restore tick (nil state reader in observe). The caller's
	// Extensions (observer / listener) attach here too.
	process.wireRuntimeDeps(processOptions.extensions)
	process.parentID = snapshot.ParentID
	process.depth = snapshot.Depth
	process.startedAt = snapshot.StartedAt

	// Re-populate state.
	process.state.transition(snapshot.Status)
	if err := process.state.restoreSuspension(snapshot.Suspension); err != nil {
		return nil, &continuationStateError{err: fmt.Errorf("suspension: %w", err)}
	}
	if err := process.restoreNestedSuspension(snapshot.Suspension); err != nil {
		return nil, &continuationStateError{err: fmt.Errorf("nested suspension: %w", err)}
	}
	if snapshot.GoalName != "" {
		for _, goal := range agent.Goals() {
			if goal.Name() == snapshot.GoalName {
				process.state.pursue(goal)
				break
			}
		}
	}
	if snapshot.Failure != nil {
		failure := *snapshot.Failure
		process.state.restoreFailure(&failure)
	}
	for _, run := range snapshot.History {
		process.state.recordActionRun(ActionRun{
			ActionName: run.ActionName,
			StartedAt:  run.StartedAt,
			Duration:   run.Duration,
			Status:     run.Status,
		})
	}

	process.budget.restore(
		snapshot.OwnCost,
		snapshot.OwnTokens,
		snapshot.OwnModelCalls,
		snapshot.OwnEmbeddingCalls,
	)

	// Re-populate blackboard when the implementation supports it. The
	// tagged values decode back to their concrete Go types via the type
	// table the agent's action I/O bindings declare (see
	// core.Agent.DecodeBlackboard) — so a restored typed-action input is the
	// original struct, not the map JSON would otherwise yield.
	bindings, objects, err := agent.DecodeBlackboard(snapshot.Blackboard, snapshot.Objects)
	if err != nil {
		return nil, &continuationStateError{err: fmt.Errorf("decode blackboard: %w", err)}
	}
	if err := restoreBlackboard(blackboard, BlackboardState{
		Bindings:   bindings,
		Conditions: snapshot.Conditions,
		Objects:    objects,
	}); err != nil {
		return nil, fmt.Errorf("restore blackboard: %w", err)
	}

	return process, nil
}
