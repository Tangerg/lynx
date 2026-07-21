package runtime

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
)

// SnapshotFailurePolicy controls automatic durability failure behavior.
type SnapshotFailurePolicy = core.SnapshotFailurePolicy

const (
	SnapshotFailureFailProcess  = core.SnapshotFailureFailProcess
	SnapshotFailurePauseProcess = core.SnapshotFailurePauseProcess
	SnapshotFailureReportOnly   = core.SnapshotFailureReportOnly
)

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

	err := p.engine.saveProcess(snapshotCtx, p, true)
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
		return err
	default:
		p.state.fail(err)
		return err
	}
}

// Save captures the named process tree into the configured [core.ProcessStore].
// It returns an error when no store is configured, the process id is unknown,
// the process is actively running, another checkpoint capture owns the same
// tree, or the store rejects the capture. Automatic snapshots use the private
// post-tick boundary and are not subject to the active-run rejection.
func (e *Engine) Save(ctx context.Context, processID string) error {
	if e.processStore == nil {
		return errors.New("runtime.Engine.Save: no ProcessStore configured")
	}
	process, ok := e.processes.get(processID)
	if !ok {
		return fmt.Errorf("runtime.Engine.Save: id %q not registered", processID)
	}
	return e.saveProcess(ctx, process, false)
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
	sequenceKey := processID
	if process, ok := e.Process(processID); ok {
		sequenceKey = e.processTreeRootID(process)
	}
	releaseSave, err := e.processSaves.acquire(ctx, sequenceKey)
	if err != nil {
		return fmt.Errorf("runtime.Engine.Discard: sequence persistence: %w", err)
	}
	defer releaseSave()
	tree, err := e.discoverProcessTrees([]string{processID})
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

func (e *Engine) saveProcess(ctx context.Context, process *Process, allowActiveRun bool) error {
	if e.processStore == nil {
		return errors.New("runtime.Engine.saveProcess: no ProcessStore configured")
	}
	ctx = normalizeContext(ctx)
	if !allowActiveRun && process.state.runActive() {
		return fmt.Errorf("runtime.Engine.saveProcess: process %q: %w", process.ID(), ErrProcessRunning)
	}
	tree, err := e.claimProcessTree(process, map[string]struct{}{}, allowActiveRun)
	if err != nil {
		return err
	}

	if err := captureClaimedProcessTree(tree); err != nil {
		releaseProcessTree(tree)
		return err
	}
	var cleanup []deferredProcessCleanup
	collectNestedChildCleanup(tree, &cleanup)
	cleanupRoots := cleanupProcessRoots(cleanup)
	releaseSave, err := e.processSaves.acquire(ctx, e.processTreeRootID(process))
	if err != nil {
		releaseProcessTree(tree)
		return fmt.Errorf("runtime.Engine.saveProcess: sequence persistence: %w", err)
	}
	defer releaseSave()
	// Capture is already done: tree.snapshot holds immutable copies. Drop the
	// checkpoint claims now so the processes may resume while the store write is
	// in flight; from here tree is a pure data carrier and must not be captured
	// again or read for live state. The processSaves lock (held via releaseSave)
	// still serializes concurrent writes of this same tree.
	releaseProcessTree(tree)

	cleanupTree, err := e.discoverProcessTrees(cleanupRoots)
	if err != nil {
		return fmt.Errorf("runtime.Engine.saveProcess: %w", err)
	}
	if err := cleanupTree.wait(ctx); err != nil {
		return fmt.Errorf("runtime.Engine.saveProcess: %w", err)
	}
	if err := cleanupTree.claim(); err != nil {
		return fmt.Errorf("runtime.Engine.saveProcess: %w", err)
	}
	err = e.saveCapturedProcessTree(ctx, tree, cleanupRoots)
	if err != nil {
		cleanupTree.releaseClaims()
		return err
	}
	for _, pending := range cleanup {
		pending.owner.acknowledgeNestedChildCleanup(pending.roots)
	}
	cleanupTree.release()
	return nil
}

func (e *Engine) processTreeRootID(process *Process) string {
	if process == nil {
		return ""
	}
	rootID := process.ID()
	visited := map[string]struct{}{rootID: {}}
	for parentID := process.ParentID(); parentID != ""; parentID = process.ParentID() {
		if _, duplicate := visited[parentID]; duplicate {
			break
		}
		parent, ok := e.Process(parentID)
		if !ok {
			break
		}
		visited[parentID] = struct{}{}
		rootID = parentID
		process = parent
	}
	return rootID
}

type claimedProcessTree struct {
	process   *Process
	relations []*nestedChildRelation
	children  []*claimedProcessTree
	snapshot  core.ProcessSnapshot
}

func (e *Engine) claimProcessTree(process *Process, visited map[string]struct{}, allowActiveRun bool) (*claimedProcessTree, error) {
	if process == nil {
		return nil, errors.New("runtime.Engine.saveProcess: process is nil")
	}
	if _, duplicate := visited[process.ID()]; duplicate {
		return nil, fmt.Errorf("%w: nested process cycle at %q", core.ErrInvalidSnapshot, process.ID())
	}
	visited[process.ID()] = struct{}{}
	if err := process.state.claimCheckpoint(allowActiveRun); err != nil {
		return nil, fmt.Errorf("runtime.Engine.saveProcess: claim process %q checkpoint: %w", process.ID(), err)
	}
	checkpoint, err := nestedChildrenFromSuspension(process.Suspension())
	if err != nil {
		process.state.releaseCheckpoint()
		return nil, err
	}

	tree := &claimedProcessTree{
		process:   process,
		relations: checkpoint.relations,
		children:  make([]*claimedProcessTree, 0, len(checkpoint.relations)),
	}
	for _, relation := range checkpoint.relations {
		child, ok := e.Process(relation.ChildID)
		if !ok {
			releaseProcessTree(tree)
			return nil, fmt.Errorf("%w: nested child process %q is missing", core.ErrInvalidSnapshot, relation.ChildID)
		}
		childTree, claimErr := e.claimProcessTree(child, visited, false)
		if claimErr != nil {
			releaseProcessTree(tree)
			return nil, fmt.Errorf("claim nested child %q: %w", child.ID(), claimErr)
		}
		tree.children = append(tree.children, childTree)
		if err := relation.validateProcess(process, child); err != nil {
			releaseProcessTree(tree)
			return nil, err
		}
	}
	return tree, nil
}

func captureClaimedProcessTree(tree *claimedProcessTree) error {
	if tree == nil || tree.process == nil {
		return errors.New("runtime.Engine.saveProcess: claimed process tree is incomplete")
	}
	snapshot, err := tree.process.snapshotClaimed()
	if err != nil {
		return err
	}
	tree.snapshot = snapshot
	for index, child := range tree.children {
		if err := captureClaimedProcessTree(child); err != nil {
			return err
		}
		if err := tree.relations[index].validateSnapshot(tree.snapshot, child.snapshot); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) saveCapturedProcessTree(ctx context.Context, tree *claimedProcessTree, deletes []string) error {
	var snapshots []core.ProcessSnapshot
	collectCapturedSnapshots(tree, &snapshots)
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

func collectCapturedSnapshots(tree *claimedProcessTree, snapshots *[]core.ProcessSnapshot) {
	if tree == nil {
		return
	}
	for _, child := range tree.children {
		collectCapturedSnapshots(child, snapshots)
	}
	*snapshots = append(*snapshots, tree.snapshot)
}

type deferredProcessCleanup struct {
	owner *Process
	roots []string
}

func collectNestedChildCleanup(tree *claimedProcessTree, cleanup *[]deferredProcessCleanup) {
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

func (e *Engine) discoverProcessTrees(roots []string) (*discoveredProcessTrees, error) {
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

func releaseProcessTree(tree *claimedProcessTree) {
	if tree == nil {
		return
	}
	for index := len(tree.children) - 1; index >= 0; index-- {
		releaseProcessTree(tree.children[index])
	}
	tree.process.state.releaseCheckpoint()
}
