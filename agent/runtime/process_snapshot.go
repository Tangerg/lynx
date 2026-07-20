package runtime

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/planning"
)

// SnapshotFailurePolicy controls automatic durability failure behavior.
type SnapshotFailurePolicy = core.SnapshotFailurePolicy

const (
	SnapshotFailureFailProcess  = core.SnapshotFailureFailProcess
	SnapshotFailurePauseProcess = core.SnapshotFailurePauseProcess
	SnapshotFailureReportOnly   = core.SnapshotFailureReportOnly
)

// ErrAtomicSnapshotUnsupported reports that a ProcessStore can save individual
// snapshots but cannot atomically commit a nested process tree.
var ErrAtomicSnapshotUnsupported = errors.New("runtime: process store does not support atomic process-tree snapshots")

// ErrResumableSnapshotLost reports that a stored process no longer contains a
// compatible waiting continuation for this Engine.
var ErrResumableSnapshotLost = errors.New("resumable process snapshot lost")

func (p *Process) maybeAutoSnapshot(ctx context.Context) error {
	if p.engine == nil || !p.engine.autoSnapshot || p.engine.processStore == nil {
		return nil
	}

	_, err := p.engine.saveProcess(ctx, p)
	if err == nil {
		return nil
	}
	_, span := agentTracer.Start(ctx, spanAutoSnapshot)
	span.SetAttributes(attribute.String(attrProcessID, p.id))
	finishSpanWithError(span, err)
	span.End()
	policy := p.engine.snapshotFailurePolicy
	p.publishEvent(ctx, event.ProcessSnapshotFailed{
		Header: p.eventHeader(),
		Policy: policy,
		Err:    err,
	})

	switch policy {
	case SnapshotFailureReportOnly:
		return nil
	case SnapshotFailurePauseProcess:
		p.state.pauseDurability()
		return nil
	default:
		p.state.failDurability(err)
		return err
	}
}

// Save captures the named process into the configured
// [core.ProcessStore] under its current id. Errors when no store is
// configured, the process id is unknown, or the store rejects the write. A
// process with durable nested children additionally requires the store to
// implement [core.SnapshotBatchWriter].
func (e *Engine) Save(ctx context.Context, processID string) (uint64, error) {
	if e.processStore == nil {
		return 0, errors.New("runtime.Engine.Save: no ProcessStore configured")
	}
	process, ok := e.processes.get(processID)
	if !ok {
		return 0, fmt.Errorf("runtime.Engine.Save: id %q not registered", processID)
	}
	return e.saveProcess(ctx, process)
}

func (e *Engine) saveProcess(ctx context.Context, process *Process) (uint64, error) {
	if e.processStore == nil {
		return 0, errors.New("runtime.Engine.saveProcess: no ProcessStore configured")
	}
	ctx = normalizeContext(ctx)
	tree, err := e.lockProcessTree(process, map[string]struct{}{})
	if err != nil {
		return 0, err
	}

	if err := captureLockedProcessTree(tree); err != nil {
		unlockProcessTree(tree)
		return 0, err
	}
	revision, err := e.saveLockedProcessTree(ctx, tree)
	if err != nil {
		unlockProcessTree(tree)
		return 0, err
	}
	var cleanup []string
	collectNestedChildCleanup(tree, &cleanup)
	unlockProcessTree(tree)
	for _, childID := range cleanup {
		e.discardProcessTree(ctx, childID)
	}
	return revision, nil
}

type lockedProcessTree struct {
	process   *Process
	relations []*nestedChildRelation
	children  []*lockedProcessTree
	snapshot  core.ProcessSnapshot
}

func (e *Engine) lockProcessTree(process *Process, visited map[string]struct{}) (*lockedProcessTree, error) {
	if process == nil {
		return nil, errors.New("runtime.Engine.saveProcess: process is nil")
	}
	if _, duplicate := visited[process.ID()]; duplicate {
		return nil, fmt.Errorf("%w: nested process cycle at %q", core.ErrInvalidSnapshot, process.ID())
	}
	visited[process.ID()] = struct{}{}
	process.checkpointMu.Lock()
	checkpoint, err := nestedChildrenFromSuspension(process.Suspension())
	if err != nil {
		process.checkpointMu.Unlock()
		return nil, err
	}

	tree := &lockedProcessTree{
		process:   process,
		relations: checkpoint.relations,
		children:  make([]*lockedProcessTree, 0, len(checkpoint.relations)),
	}
	for _, relation := range checkpoint.relations {
		child, ok := e.Process(relation.ChildID)
		if !ok {
			unlockProcessTree(tree)
			return nil, fmt.Errorf("%w: nested child process %q is missing", core.ErrInvalidSnapshot, relation.ChildID)
		}
		childTree, lockErr := e.lockProcessTree(child, visited)
		if lockErr != nil {
			unlockProcessTree(tree)
			return nil, fmt.Errorf("lock nested child %q: %w", child.ID(), lockErr)
		}
		tree.children = append(tree.children, childTree)
		if err := relation.validateProcess(process, child); err != nil {
			unlockProcessTree(tree)
			return nil, err
		}
	}
	return tree, nil
}

func captureLockedProcessTree(tree *lockedProcessTree) error {
	if tree == nil || tree.process == nil {
		return errors.New("runtime.Engine.saveProcess: locked process tree is incomplete")
	}
	snapshot, err := tree.process.snapshot()
	if err != nil {
		return err
	}
	tree.snapshot = snapshot
	for index, child := range tree.children {
		if err := captureLockedProcessTree(child); err != nil {
			return err
		}
		if err := tree.relations[index].validateSnapshot(tree.snapshot, child.snapshot); err != nil {
			return err
		}
	}
	return nil
}

type lockedSnapshotWrite struct {
	tree     *lockedProcessTree
	snapshot core.ProcessSnapshot
}

func (e *Engine) saveLockedProcessTree(ctx context.Context, tree *lockedProcessTree) (uint64, error) {
	var writes []lockedSnapshotWrite
	collectLockedSnapshotWrites(tree, &writes)
	if len(writes) == 0 {
		return 0, errors.New("runtime.Engine.saveProcess: captured process tree is empty")
	}
	for _, pending := range writes {
		actual := pending.tree.process.state.snapshotRevision()
		if actual != pending.snapshot.Revision {
			return 0, &core.RevisionConflictError{
				ProcessID: pending.snapshot.ID,
				Expected:  pending.snapshot.Revision,
				Actual:    actual,
			}
		}
		if pending.snapshot.Revision == math.MaxUint64 {
			return 0, fmt.Errorf("runtime.Engine.saveProcess: %w: process %q revision is exhausted",
				core.ErrInvalidSnapshot, pending.snapshot.ID)
		}
	}

	if err := e.commitSnapshotWrites(ctx, writes); err != nil {
		return 0, err
	}
	for _, pending := range writes {
		pending.tree.process.state.commitRevision(pending.snapshot.Revision + 1)
	}
	return tree.snapshot.Revision + 1, nil
}

func collectLockedSnapshotWrites(tree *lockedProcessTree, writes *[]lockedSnapshotWrite) {
	if tree == nil {
		return
	}
	for _, child := range tree.children {
		collectLockedSnapshotWrites(child, writes)
	}
	*writes = append(*writes, lockedSnapshotWrite{
		tree:     tree,
		snapshot: tree.snapshot,
	})
}

func (e *Engine) commitSnapshotWrites(ctx context.Context, writes []lockedSnapshotWrite) error {
	if len(writes) == 1 {
		return e.processStore.Save(ctx, writes[0].snapshot)
	}
	batchWriter, ok := e.processStore.(core.SnapshotBatchWriter)
	if !ok {
		return fmt.Errorf("%w: store %T cannot commit %d snapshots", ErrAtomicSnapshotUnsupported, e.processStore, len(writes))
	}
	batch := make([]core.ProcessSnapshot, len(writes))
	for index, pending := range writes {
		batch[index] = pending.snapshot
	}
	return batchWriter.SaveBatch(ctx, batch)
}

func collectNestedChildCleanup(tree *lockedProcessTree, cleanup *[]string) {
	if tree == nil {
		return
	}
	*cleanup = append(*cleanup, tree.process.takeNestedChildCleanup()...)
	for _, child := range tree.children {
		collectNestedChildCleanup(child, cleanup)
	}
}

func unlockProcessTree(tree *lockedProcessTree) {
	if tree == nil {
		return
	}
	for index := len(tree.children) - 1; index >= 0; index-- {
		unlockProcessTree(tree.children[index])
	}
	tree.process.checkpointMu.Unlock()
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
	if e.processStore == nil {
		return nil, errors.New("runtime.Engine.Restore: no ProcessStore configured")
	}
	snapshot, err := e.processStore.Load(ctx, processID)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.Restore: %w", err)
	}
	return e.RestoreSnapshot(snapshot, options)
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
				e.processes.replace(restored[index].previous)
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

	blackboardState, err := snapshotBlackboard(p.blackboard)
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
	blackboard, err := e.resolveBlackboard(options.Blackboard)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.RestoreSnapshot: %w", err)
	}
	planner, err := e.resolvePlanner(agent, processOptions.extensions)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.RestoreSnapshot: %w", err)
	}
	domain, err := planning.DomainForAgent(agent)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.RestoreSnapshot: domain: %w", err)
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
	bindings, objects, err := agent.DecodeBlackboard(snapshot.Blackboard, snapshot.Objects)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.RestoreSnapshot: decode blackboard: %w", err)
	}
	if err := restoreBlackboard(blackboard, BlackboardState{
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
