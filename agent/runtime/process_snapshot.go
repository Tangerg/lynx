package runtime

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
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

// ErrResumableSnapshotLost reports that a stored process no longer contains a
// compatible waiting continuation for this Engine.
var ErrResumableSnapshotLost = errors.New("resumable process snapshot lost")

func (p *Process) maybeAutoSnapshot(ctx context.Context) error {
	if p.engine == nil || !p.engine.autoSnapshot || p.engine.processStore == nil {
		return nil
	}

	// Persist runtime state independently of the request that drove it. In
	// particular, cancellation is itself a state transition worth recording.
	// The engine-owned deadline prevents a stuck store from retaining the run
	// loop indefinitely.
	snapshotCtx, cancel := context.WithTimeout(
		context.WithoutCancel(normalizeContext(ctx)),
		p.engine.snapshotFinalizeTimeout,
	)
	defer cancel()

	err := p.engine.saveProcess(snapshotCtx, p)
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
		if p.state.pauseDurability() {
			return nil
		}
		return err
	default:
		p.state.fail(err)
		return err
	}
}

// Save captures the named process tree into the configured [core.ProcessStore].
// It returns an error when no store is configured, the process id is unknown,
// or the store rejects the capture.
func (e *Engine) Save(ctx context.Context, processID string) error {
	if e.processStore == nil {
		return errors.New("runtime.Engine.Save: no ProcessStore configured")
	}
	process, ok := e.processes.get(processID)
	if !ok {
		return fmt.Errorf("runtime.Engine.Save: id %q not registered", processID)
	}
	return e.saveProcess(ctx, process)
}

// Discard terminates a process tree, waits for every active run to release its
// final-snapshot ownership, asks the configured store to delete its durable
// tree, and removes the live processes from the registry in descendant-first
// order.
func (e *Engine) Discard(ctx context.Context, processID string) error {
	if e == nil {
		return errors.New("runtime.Engine.Discard: nil Engine")
	}
	ctx = normalizeContext(ctx)
	tree, err := e.discoverProcessTrees(ctx, []string{processID})
	if err != nil {
		return fmt.Errorf("runtime.Engine.Discard: %w", err)
	}
	var terminateErrs []error
	for _, id := range tree.order {
		process := tree.live[id]
		if process == nil || process.Status().IsTerminal() {
			continue
		}
		if err := e.Kill(ctx, id); err != nil {
			terminateErrs = append(terminateErrs, fmt.Errorf("runtime.Engine.Discard: terminate process %q: %w", id, err))
		}
	}
	if err := tree.wait(ctx); err != nil {
		return errors.Join(errors.Join(terminateErrs...), fmt.Errorf("runtime.Engine.Discard: %w", err))
	}
	if err := tree.claim(); err != nil {
		return errors.Join(errors.Join(terminateErrs...), fmt.Errorf("runtime.Engine.Discard: %w", err))
	}
	if e.processStore != nil {
		change := core.ProcessSnapshotChange{DeleteRoots: []string{processID}}
		if err := e.processStore.Apply(ctx, change); err != nil {
			tree.releaseClaims()
			return errors.Join(errors.Join(terminateErrs...), fmt.Errorf("runtime.Engine.Discard: delete snapshots: %w", err))
		}
	}
	tree.release()
	return errors.Join(terminateErrs...)
}

func (e *Engine) saveProcess(ctx context.Context, process *Process) error {
	if e.processStore == nil {
		return errors.New("runtime.Engine.saveProcess: no ProcessStore configured")
	}
	ctx = normalizeContext(ctx)
	tree, err := e.lockProcessTree(process, map[string]struct{}{})
	if err != nil {
		return err
	}

	if err := captureLockedProcessTree(tree); err != nil {
		unlockProcessTree(tree)
		return err
	}
	var cleanup []deferredProcessCleanup
	collectNestedChildCleanup(tree, &cleanup)
	cleanupRoots := cleanupProcessRoots(cleanup)
	cleanupTree, err := e.discoverProcessTrees(ctx, cleanupRoots)
	if err != nil {
		unlockProcessTree(tree)
		return err
	}
	if err := cleanupTree.wait(ctx); err != nil {
		unlockProcessTree(tree)
		return err
	}
	if err := cleanupTree.claim(); err != nil {
		unlockProcessTree(tree)
		return err
	}
	err = e.saveLockedProcessTree(ctx, tree, cleanupRoots)
	if err != nil {
		cleanupTree.releaseClaims()
		unlockProcessTree(tree)
		return err
	}
	for _, pending := range cleanup {
		pending.owner.acknowledgeNestedChildCleanup(pending.roots)
	}
	unlockProcessTree(tree)
	cleanupTree.release()
	return nil
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

func (e *Engine) saveLockedProcessTree(ctx context.Context, tree *lockedProcessTree, deletes []string) error {
	var snapshots []core.ProcessSnapshot
	collectLockedSnapshots(tree, &snapshots)
	if len(snapshots) == 0 {
		return errors.New("runtime.Engine.saveProcess: captured process tree is empty")
	}
	change := core.ProcessSnapshotChange{
		Tree: &core.ProcessSnapshotTree{
			RootID:    tree.process.ID(),
			Snapshots: snapshots,
		},
		DeleteRoots: slices.Clone(deletes),
	}
	return e.processStore.Apply(ctx, change)
}

func collectLockedSnapshots(tree *lockedProcessTree, snapshots *[]core.ProcessSnapshot) {
	if tree == nil {
		return
	}
	for _, child := range tree.children {
		collectLockedSnapshots(child, snapshots)
	}
	*snapshots = append(*snapshots, tree.snapshot)
}

type deferredProcessCleanup struct {
	owner *Process
	roots []string
}

func collectNestedChildCleanup(tree *lockedProcessTree, cleanup *[]deferredProcessCleanup) {
	if tree == nil {
		return
	}
	if roots := tree.process.nestedChildCleanupSnapshot(); len(roots) > 0 {
		*cleanup = append(*cleanup, deferredProcessCleanup{owner: tree.process, roots: roots})
	}
	for _, child := range tree.children {
		collectNestedChildCleanup(child, cleanup)
	}
}

func cleanupProcessRoots(cleanup []deferredProcessCleanup) []string {
	set := make(map[string]struct{})
	for _, pending := range cleanup {
		for _, root := range pending.roots {
			set[root] = struct{}{}
		}
	}
	return slices.Sorted(maps.Keys(set))
}

type discoveredProcessTrees struct {
	engine  *Engine
	order   []string
	live    map[string]*Process
	claimed []*Process
}

func (e *Engine) discoverProcessTrees(ctx context.Context, roots []string) (*discoveredProcessTrees, error) {
	ctx = normalizeContext(ctx)
	discovered := &discoveredProcessTrees{engine: e, live: make(map[string]*Process)}
	if len(roots) == 0 {
		return discovered, nil
	}
	children := make(map[string]map[string]struct{})
	parents := make(map[string]string)
	addChild := func(parentID, childID string) error {
		if parentID == "" {
			return nil
		}
		if parentID == childID {
			return fmt.Errorf("runtime: discover process cleanup: process %q is its own parent", childID)
		}
		if previous, exists := parents[childID]; exists && previous != parentID {
			return fmt.Errorf("runtime: discover process cleanup: process %q has parent %q, already linked to %q", childID, parentID, previous)
		}
		parents[childID] = parentID
		if children[parentID] == nil {
			children[parentID] = make(map[string]struct{})
		}
		children[parentID][childID] = struct{}{}
		return nil
	}
	for _, process := range e.Processes() {
		if process == nil {
			return nil, errors.New("runtime: discover process cleanup: registry contains nil process")
		}
		discovered.live[process.ID()] = process
		if err := addChild(process.ParentID(), process.ID()); err != nil {
			return nil, err
		}
	}
	visitState := make(map[string]uint8)
	var walk func(string) error
	walk = func(id string) error {
		switch visitState[id] {
		case 1:
			return fmt.Errorf("runtime: discover process cleanup: descendant cycle reaches %q", id)
		case 2:
			return nil
		}
		visitState[id] = 1
		childIDs := slices.Sorted(maps.Keys(children[id]))
		for _, childID := range childIDs {
			if err := walk(childID); err != nil {
				return err
			}
		}
		visitState[id] = 2
		discovered.order = append(discovered.order, id)
		return nil
	}
	sortedRoots := slices.Clone(roots)
	slices.Sort(sortedRoots)
	for _, root := range sortedRoots {
		if strings.TrimSpace(root) == "" || strings.TrimSpace(root) != root {
			return nil, fmt.Errorf("runtime: discover process cleanup: invalid root process ID %q", root)
		}
		if err := walk(root); err != nil {
			return nil, err
		}
	}
	return discovered, nil
}

func (tree *discoveredProcessTrees) wait(ctx context.Context) error {
	if tree == nil {
		return nil
	}
	var errs []error
	for _, id := range tree.order {
		process := tree.live[id]
		if process == nil {
			continue
		}
		if !process.Status().IsTerminal() {
			errs = append(errs, fmt.Errorf("process %q: %w", id, ErrProcessActive))
			continue
		}
		if err := process.state.waitRun(ctx); err != nil {
			errs = append(errs, fmt.Errorf("wait for process %q: %w", id, err))
		}
	}
	return errors.Join(errs...)
}

func (tree *discoveredProcessTrees) claim() error {
	if tree == nil {
		return nil
	}
	for _, id := range tree.order {
		process := tree.live[id]
		if process == nil {
			continue
		}
		if !process.state.claimRemoval() {
			tree.releaseClaims()
			return fmt.Errorf("claim process %q removal: %w", id, ErrProcessActive)
		}
		tree.claimed = append(tree.claimed, process)
		if current, exists := tree.engine.Process(id); !exists || current != process {
			tree.releaseClaims()
			return fmt.Errorf("claim process %q removal: %w", id, ErrProcessNotFound)
		}
	}
	return nil
}

func (tree *discoveredProcessTrees) releaseClaims() {
	if tree == nil {
		return
	}
	for _, process := range tree.claimed {
		process.state.releaseRemoval()
	}
	tree.claimed = nil
}

func (tree *discoveredProcessTrees) release() {
	if tree == nil || tree.engine == nil {
		return
	}
	for _, process := range tree.claimed {
		tree.engine.processes.unregisterClaimedLeaf(process)
		process.state.releaseRemoval()
	}
	tree.claimed = nil
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
			e.processes.unregister(restored[index].process)
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
	if snapshot.ID != processID {
		return core.ProcessSnapshot{}, fmt.Errorf("%w: stored snapshot identity does not match process %q", core.ErrInvalidSnapshot, processID), nil
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
	extensions, err := parent.childExtensions(options.Extensions)
	if err != nil {
		return core.ProcessOptions{}, fmt.Errorf("restore child extensions: %w", err)
	}
	options.Extensions = extensions
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
	state := p.captureDurableState()
	snapshot := core.ProcessSnapshot{
		SchemaVersion:     core.ProcessSnapshotSchemaVersion,
		ID:                p.ID(),
		ParentID:          p.ParentID(),
		Depth:             p.depth,
		Deployment:        p.Deployment(),
		StartedAt:         p.StartedAt(),
		CapturedAt:        time.Now(),
		Status:            state.status,
		Suspension:        state.suspension,
		OwnCost:           state.ownCost,
		OwnTokens:         state.ownTokens,
		OwnModelCalls:     state.modelCalls,
		OwnEmbeddingCalls: state.embeddingCalls,
	}

	if goal := state.goal; goal != nil {
		snapshot.GoalName = goal.Name()
	}
	if state.failure != nil {
		snapshot.Failure = state.failure.Error()
	}
	history := state.history
	if len(history) > 0 {
		snapshot.History = make([]core.ActionRunSnapshot, len(history))
		for i, run := range history {
			snapshot.History[i] = core.ActionRunSnapshot{
				ActionName: run.ActionName,
				StartedAt:  run.StartedAt,
				Duration:   run.Duration,
				Status:     run.Status,
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

type durableProcessState struct {
	status         core.ProcessStatus
	goal           *core.Goal
	failure        error
	suspension     *interaction.Suspension
	history        []ActionRun
	ownCost        float64
	ownTokens      int
	modelCalls     []core.ModelCall
	embeddingCalls []core.EmbeddingCall
}

func (p *Process) captureDurableState() durableProcessState {
	p.state.mu.RLock()
	defer p.state.mu.RUnlock()
	var suspension *interaction.Suspension
	if p.state.pendingSuspension != nil {
		suspension = p.state.pendingSuspension.Clone()
	}
	return durableProcessState{
		status:         p.state.currentStatus,
		goal:           p.state.currentGoal,
		failure:        p.state.runErr,
		suspension:     suspension,
		history:        slices.Clone(p.state.history),
		ownCost:        p.budget.ownCost,
		ownTokens:      p.budget.ownTokens,
		modelCalls:     slices.Clone(p.budget.modelCalls),
		embeddingCalls: slices.Clone(p.budget.embeddingCalls),
	}
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
		process.state.restoreFailure(errors.New(snapshot.Failure))
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
