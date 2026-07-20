package runtime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/planning"
)

// ErrResumableSnapshotLost reports that a stored process no longer contains a
// compatible waiting continuation for this Engine.
var ErrResumableSnapshotLost = errors.New("resumable process snapshot lost")

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
	_, loss, err := e.loadResumableTree(ctx, processID, map[string]struct{}{}, true)
	if err != nil {
		return false, err
	}
	return loss == nil, nil
}

// RestoreResumable loads and rebuilds a waiting continuation. Every durable
// state loss or incompatibility wraps ErrResumableSnapshotLost; persistence
// access failures remain ordinary errors so hosts can distinguish an unusable
// continuation from a temporarily unavailable store.
func (e *Engine) RestoreResumable(ctx context.Context, processID string, options core.ProcessOptions) (*Process, error) {
	tree, loss, err := e.loadResumableTree(ctx, processID, map[string]struct{}{}, true)
	if err != nil {
		return nil, err
	}
	if loss != nil {
		return nil, fmt.Errorf("runtime.Engine.RestoreResumable: %w: %w", ErrResumableSnapshotLost, loss)
	}
	var (
		restored []restoredProcess
		links    []restoredProcessLink
	)
	process, err := e.restoreResumableTree(ctx, tree, options, nil, &restored, &links)
	if err != nil {
		for index := len(links) - 1; index >= 0; index-- {
			links[index].parent.budget.removeChild(links[index].child)
		}
		for index := len(restored) - 1; index >= 0; index-- {
			e.processes.unregister(restored[index].process.ID())
			if restored[index].previous != nil {
				e.processes.register(restored[index].previous)
			}
		}
		return nil, fmt.Errorf("runtime.Engine.RestoreResumable: %w: rebuild process %q: %w", ErrResumableSnapshotLost, processID, err)
	}
	return process, nil
}

type resumableProcessTree struct {
	snapshot core.ProcessSnapshot
	children []*resumableProcessTree
}

func (e *Engine) loadResumableTree(
	ctx context.Context,
	processID string,
	visited map[string]struct{},
	root bool,
) (*resumableProcessTree, error, error) {
	if _, duplicate := visited[processID]; duplicate {
		return nil, fmt.Errorf("%w: nested process cycle at %q", core.ErrInvalidSnapshot, processID), nil
	}
	visited[processID] = struct{}{}
	snapshot, loss, err := e.loadStoredSnapshot(ctx, processID)
	if err != nil || loss != nil {
		return nil, loss, err
	}
	if root {
		if err := ValidateResumableSnapshot(snapshot); err != nil {
			return nil, err, nil
		}
	} else if snapshot.Status != core.StatusWaiting && !snapshot.Status.IsTerminal() {
		return nil, fmt.Errorf("%w: nested child %q has non-resumable status %s", core.ErrInvalidSnapshot, processID, snapshot.Status), nil
	}
	if _, ok := e.catalog.lookup(snapshot.Deployment); !ok {
		return nil, fmt.Errorf("%w: %s", ErrDeploymentNotFound, snapshot.Deployment), nil
	}

	tree := &resumableProcessTree{snapshot: snapshot}
	checkpoint, relationErr := nestedChildrenFromSuspension(snapshot.Suspension)
	if relationErr != nil {
		return nil, relationErr, nil
	}
	tree.children = make([]*resumableProcessTree, 0, len(checkpoint.relations))
	for _, relation := range checkpoint.relations {
		childTree, childLoss, childErr := e.loadResumableTree(ctx, relation.ChildID, visited, false)
		if childErr != nil || childLoss != nil {
			return nil, childLoss, childErr
		}
		if err := relation.validateSnapshot(snapshot, childTree.snapshot); err != nil {
			return nil, err, nil
		}
		tree.children = append(tree.children, childTree)
	}
	return tree, nil, nil
}

func (e *Engine) loadStoredSnapshot(ctx context.Context, processID string) (core.ProcessSnapshot, error, error) {
	if e == nil {
		return core.ProcessSnapshot{}, nil, errors.New("runtime.Engine.Resumable: nil engine")
	}
	if e.processStore == nil {
		return core.ProcessSnapshot{}, nil, errors.New("runtime.Engine.Resumable: no ProcessStore configured")
	}
	snapshot, err := e.processStore.Load(ctx, processID)
	if err != nil {
		if errors.Is(err, core.ErrSnapshotNotFound) ||
			errors.Is(err, core.ErrSnapshotSchema) ||
			errors.Is(err, core.ErrInvalidSnapshot) {
			return core.ProcessSnapshot{}, err, nil
		}
		return core.ProcessSnapshot{}, nil, fmt.Errorf("runtime.Engine.Resumable: load process %q: %w", processID, err)
	}
	if snapshot.ID != processID || snapshot.Revision == 0 {
		return core.ProcessSnapshot{}, fmt.Errorf("%w: stored snapshot identity/revision does not match process %q", core.ErrInvalidSnapshot, processID), nil
	}
	if err := snapshot.Validate(); err != nil {
		return core.ProcessSnapshot{}, err, nil
	}
	return snapshot, nil, nil
}

type restoredProcessLink struct {
	parent *Process
	child  *Process
}

type restoredProcess struct {
	process  *Process
	previous *Process
}

func (e *Engine) restoreResumableTree(
	ctx context.Context,
	tree *resumableProcessTree,
	options core.ProcessOptions,
	parent *Process,
	restored *[]restoredProcess,
	links *[]restoredProcessLink,
) (*Process, error) {
	if tree == nil {
		return nil, errors.New("runtime: resumable process tree is nil")
	}
	snapshot := tree.snapshot
	previous, _ := e.Process(snapshot.ID)
	process, err := e.RestoreSnapshot(snapshot, options)
	if err != nil {
		return nil, err
	}
	*restored = append(*restored, restoredProcess{process: process, previous: previous})
	if parent != nil {
		linker := childRun{ctx: ctx, engine: e}
		if err := linker.restoreSession(process, parent); err != nil {
			return nil, fmt.Errorf("restore child session: %w", err)
		}
		parent.budget.addChild(process)
		*links = append(*links, restoredProcessLink{parent: parent, child: process})
	}
	for _, childTree := range tree.children {
		childOptions, err := restoredChildOptions(ctx, process, e, childTree.snapshot.Deployment)
		if err != nil {
			return nil, err
		}
		if _, err := e.restoreResumableTree(ctx, childTree, childOptions, process, restored, links); err != nil {
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
	options.Extensions = parent.childExtensions(options.Extensions)
	return options, nil
}

// Snapshot captures the process's state into a portable
// [core.ProcessSnapshot] suitable for handing to a [core.ProcessStore].
// It waits for any active tick or suspension response to reach a framework
// checkpoint boundary, then captures one internally consistent state.
// No external state is mutated.
//
// Blackboard capture is strict: the blackboard must expose
// [BlackboardSnapshotter], every durable value must be declared and JSON-safe,
// and invalid durable state returns an error.
func (p *Process) Snapshot() (core.ProcessSnapshot, error) {
	if p == nil {
		return core.ProcessSnapshot{}, errors.New("runtime.Process.Snapshot: nil process")
	}
	p.checkpointMu.RLock()
	defer p.checkpointMu.RUnlock()
	return p.snapshot()
}

func (p *Process) snapshot() (core.ProcessSnapshot, error) {
	ownCost, ownTokens, ownModelCalls, ownEmbeddingCalls := p.budget.ownSnapshot()
	snapshot := core.ProcessSnapshot{
		SchemaVersion:     core.ProcessSnapshotSchemaVersion,
		Revision:          p.state.snapshotRevision(),
		ID:                p.ID(),
		ParentID:          p.ParentID(),
		Depth:             p.depth,
		Deployment:        p.Deployment(),
		StartedAt:         p.StartedAt(),
		CapturedAt:        time.Now(),
		Status:            p.Status(),
		Suspension:        p.Suspension(),
		OwnCost:           ownCost,
		OwnTokens:         ownTokens,
		OwnModelCalls:     ownModelCalls,
		OwnEmbeddingCalls: ownEmbeddingCalls,
	}

	if goal := p.Goal(); goal != nil {
		snapshot.GoalName = goal.Name()
	}
	if err := p.Failure(); err != nil {
		snapshot.Failure = err.Error()
	}
	history := p.History()
	if len(history) > 0 {
		snapshot.History = make([]core.ActionRunSnapshot, len(history))
		for i, run := range history {
			snapshot.History[i] = core.ActionRunSnapshot{
				ActionName: run.ActionName,
				StartedAt:  run.StartedAt,
				Duration:   run.Duration,
				Status:     run.Status,
				Attempts:   run.Attempts,
			}
		}
	}

	snapshotter, ok := p.blackboard.(BlackboardSnapshotter)
	if !ok {
		return core.ProcessSnapshot{}, errors.New("runtime.Process.Snapshot: blackboard does not support durable capture")
	}
	blackboardState, err := snapshotter.Snapshot()
	if err != nil {
		return core.ProcessSnapshot{}, fmt.Errorf("runtime.Process.Snapshot: capture blackboard: %w", err)
	}
	snapshot.Blackboard, snapshot.Objects, err = p.agent().EncodeBlackboard(blackboardState.Bindings, blackboardState.Objects)
	if err != nil {
		return core.ProcessSnapshot{}, fmt.Errorf("runtime.Process.Snapshot: encode blackboard: %w", err)
	}
	snapshot.Conditions = blackboardState.Conditions
	if err := snapshot.Validate(); err != nil {
		return core.ProcessSnapshot{}, fmt.Errorf("runtime.Process.Snapshot: %w", err)
	}
	return snapshot, nil
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
func (e *Engine) RestoreSnapshot(snapshot core.ProcessSnapshot, options core.ProcessOptions) (*Process, error) {
	if e == nil {
		return nil, errors.New("runtime.Engine.RestoreSnapshot: nil engine")
	}
	if err := snapshot.Validate(); err != nil {
		return nil, fmt.Errorf("runtime.Engine.RestoreSnapshot: %w", err)
	}

	deployment, ok := e.catalog.lookup(snapshot.Deployment)
	if !ok {
		return nil, fmt.Errorf("runtime.Engine.RestoreSnapshot: %w: %s", ErrDeploymentNotFound, snapshot.Deployment)
	}
	agent := deployment.agent

	processOptions, err := snapshotProcessOptions(options)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.RestoreSnapshot: %w", err)
	}
	dependencies, err := e.prepareProcessDependencies(options.Dependencies)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.RestoreSnapshot: %w", err)
	}
	blackboard := e.resolveBlackboard(options.Blackboard)
	planner, err := e.resolvePlanner(agent, processOptions.extensions)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.RestoreSnapshot: %w", err)
	}
	domain := planning.DomainForAgent(agent)

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
	process.state.restoreRevision(snapshot.Revision)
	if err := process.state.restoreSuspension(snapshot.Suspension); err != nil {
		return nil, fmt.Errorf("runtime.Engine.RestoreSnapshot: suspension: %w", err)
	}
	if err := process.restoreNestedSuspension(snapshot.Suspension); err != nil {
		return nil, fmt.Errorf("runtime.Engine.RestoreSnapshot: nested suspension: %w", err)
	}
	if snapshot.GoalName != "" {
		for _, goal := range agent.Goals() {
			if goal.Name() == snapshot.GoalName {
				process.state.pursue(goal)
				break
			}
		}
	}
	if snapshot.Failure != "" {
		process.state.recordFailure(errors.New(snapshot.Failure))
	}
	for _, run := range snapshot.History {
		process.state.recordActionRun(ActionRun{
			ActionName: run.ActionName,
			StartedAt:  run.StartedAt,
			Duration:   run.Duration,
			Status:     run.Status,
			Attempts:   run.Attempts,
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
	restorer, ok := blackboard.(BlackboardRestorer)
	if !ok {
		return nil, errors.New("runtime.Engine.RestoreSnapshot: blackboard does not support durable restore")
	}
	bindings, objects, err := agent.DecodeBlackboard(snapshot.Blackboard, snapshot.Objects)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.RestoreSnapshot: decode blackboard: %w", err)
	}
	if err := restorer.Restore(BlackboardState{
		Bindings:   bindings,
		Conditions: snapshot.Conditions,
		Objects:    objects,
	}); err != nil {
		return nil, fmt.Errorf("runtime.Engine.RestoreSnapshot: restore blackboard: %w", err)
	}

	// Restore keeps the snapshot's ORIGINAL process id, so refuse to clobber an
	// id still held by a live process (e.g. an auto-snapshot re-restoring while
	// the original ticks) — that would split the id across two objects. A
	// terminal / absent slot replaces cleanly.
	if !e.processes.registerNew(process) {
		return nil, fmt.Errorf("runtime: cannot restore process %s: a live process with that id is already running", process.id)
	}
	return process, nil
}
