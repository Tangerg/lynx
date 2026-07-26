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
	"go.opentelemetry.io/otel/trace"

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
	return p.autoSnapshot(ctx)
}

func (p *Process) autoSnapshot(ctx context.Context) error {
	ctx = normalizeContext(ctx)
	deadline := time.Now().Add(p.engine.snapshotFinalizeTimeout)
	if callerDeadline, ok := ctx.Deadline(); ok && callerDeadline.Before(deadline) {
		deadline = callerDeadline
	}
	snapshotCtx, cancel := context.WithDeadline(context.WithoutCancel(ctx), deadline)
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

// DiscardResult reports whether [Engine.Discard] relinquished the runtime
// process tree. Released may be true together with an error when durable
// snapshot deletion failed after the registry release became irrevocable.
type DiscardResult struct {
	Released bool
}

// Discard terminates a process tree, deletes its durable snapshots, and
// relinquishes its runtime ownership. The result distinguishes failures that
// retain runtime ownership from a durable deletion failure reported after
// release.
func (e *Engine) Discard(ctx context.Context, processID string) (DiscardResult, error) {
	ctx, span := agentTracer.Start(
		normalizeContext(ctx),
		spanDiscard,
		trace.WithAttributes(attribute.String(attrProcessID, processID)),
	)
	var (
		result     DiscardResult
		discardErr error
	)
	defer func() {
		finishSpanWithError(span, discardErr)
		span.End()
	}()
	result, discardErr = e.discard(ctx, processID, span)
	return result, discardErr
}

func (e *Engine) discard(ctx context.Context, processID string, span trace.Span) (DiscardResult, error) {
	if e == nil {
		return DiscardResult{}, errors.New("runtime.Engine.Discard: nil Engine")
	}
	sequenceKey := processID
	if process, ok := e.Process(processID); ok {
		sequenceKey = e.processTreeRootID(process)
	}
	tree, err := e.discoverProcessTrees([]string{processID})
	if err != nil {
		return DiscardResult{}, fmt.Errorf("runtime.Engine.Discard: %w", err)
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
	tree, err = e.discoverProcessTrees([]string{processID})
	if err != nil {
		return DiscardResult{}, errors.Join(errors.Join(terminateErrs...), fmt.Errorf("runtime.Engine.Discard: refresh process tree: %w", err))
	}
	if process := tree.live[processID]; process != nil {
		sequenceKey = e.processTreeRootID(process)
	}
	if err := tree.wait(ctx); err != nil {
		return DiscardResult{}, errors.Join(errors.Join(terminateErrs...), fmt.Errorf("runtime.Engine.Discard: %w", err))
	}
	releaseMutation, err := e.processMutations.acquire(ctx, sequenceKey)
	if err != nil {
		return DiscardResult{}, errors.Join(errors.Join(terminateErrs...), fmt.Errorf("runtime.Engine.Discard: acquire process tree: %w", err))
	}
	if err := tree.claim(); err != nil {
		releaseMutation()
		return DiscardResult{}, errors.Join(errors.Join(terminateErrs...), fmt.Errorf("runtime.Engine.Discard: %w", err))
	}
	releaseWrite, err := e.processWrites.acquire(ctx, sequenceKey)
	if err != nil {
		tree.releaseClaims()
		releaseMutation()
		return DiscardResult{}, errors.Join(errors.Join(terminateErrs...), fmt.Errorf("runtime.Engine.Discard: order durable delete: %w", err))
	}
	releaseMutation()
	defer releaseWrite()
	var deleteErr error
	if e.processStore != nil {
		change := core.ProcessSnapshotChange{DeleteRoots: []string{processID}}
		if err := e.processStore.Apply(ctx, change); err != nil {
			deleteErr = fmt.Errorf("runtime.Engine.Discard: delete snapshots: %w", err)
		}
	}
	if err := tree.release(); err != nil {
		return DiscardResult{}, errors.Join(deleteErr, fmt.Errorf("runtime.Engine.Discard: %w", err))
	}
	recordDiscardDiagnostics(span, terminateErrs)
	return DiscardResult{Released: true}, deleteErr
}

func recordDiscardDiagnostics(span trace.Span, diagnostics []error) {
	for _, diagnostic := range diagnostics {
		span.RecordError(diagnostic)
	}
}

func (e *Engine) saveProcess(ctx context.Context, process *Process, allowActiveRun bool) error {
	if e.processStore == nil {
		return errors.New("runtime.Engine.saveProcess: no ProcessStore configured")
	}
	ctx = normalizeContext(ctx)
	treeID := e.processTreeRootID(process)
	releaseMutation, err := e.processMutations.acquire(ctx, treeID)
	if err != nil {
		return fmt.Errorf("runtime.Engine.saveProcess: acquire process tree: %w", err)
	}
	if !e.processes.available(process) {
		releaseMutation()
		return fmt.Errorf("runtime.Engine.saveProcess: process %q: %w", process.ID(), ErrProcessNotFound)
	}
	prepared, err := e.prepareProcessSaveOwned(ctx, process, allowActiveRun)
	if err != nil {
		releaseMutation()
		return err
	}
	releaseWrite, err := e.processWrites.acquire(ctx, treeID)
	if err != nil {
		prepared.cleanupTree.releaseClaims()
		releaseMutation()
		return fmt.Errorf("runtime.Engine.saveProcess: order durable commit: %w", err)
	}
	releaseMutation()
	defer releaseWrite()
	return e.commitPreparedProcessSave(ctx, prepared)
}

type preparedProcessSave struct {
	tree         *claimedProcessTree
	cleanup      []deferredProcessCleanup
	cleanupRoots []string
	cleanupTree  *discoveredProcessTrees
}

func (e *Engine) prepareProcessSaveOwned(
	ctx context.Context,
	process *Process,
	allowActiveRun bool,
) (*preparedProcessSave, error) {
	if !allowActiveRun && process.state.runActive() {
		return nil, fmt.Errorf("runtime.Engine.saveProcess: process %q: %w", process.ID(), ErrProcessRunning)
	}
	tree, err := e.claimProcessTree(process, map[string]struct{}{}, allowActiveRun)
	if err != nil {
		return nil, err
	}

	if err := captureClaimedProcessTree(tree); err != nil {
		releaseProcessTree(tree)
		return nil, err
	}
	var cleanup []deferredProcessCleanup
	collectNestedChildCleanup(tree, &cleanup)
	cleanupRoots := cleanupProcessRoots(cleanup)
	// Capture is complete and tree.snapshot now holds immutable copies.
	releaseProcessTree(tree)

	cleanupTree, err := e.discoverProcessTrees(cleanupRoots)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.saveProcess: %w", err)
	}
	if err := cleanupTree.wait(ctx); err != nil {
		return nil, fmt.Errorf("runtime.Engine.saveProcess: %w", err)
	}
	if err := cleanupTree.claim(); err != nil {
		return nil, fmt.Errorf("runtime.Engine.saveProcess: %w", err)
	}
	return &preparedProcessSave{
		tree:         tree,
		cleanup:      cleanup,
		cleanupRoots: cleanupRoots,
		cleanupTree:  cleanupTree,
	}, nil
}

func (e *Engine) commitPreparedProcessSave(ctx context.Context, prepared *preparedProcessSave) error {
	if err := e.saveCapturedProcessTree(ctx, prepared.tree, prepared.cleanupRoots); err != nil {
		prepared.cleanupTree.releaseClaims()
		return err
	}
	if err := prepared.cleanupTree.release(); err != nil {
		return fmt.Errorf("runtime.Engine.saveProcess: release deleted process tree: %w", err)
	}
	for _, pending := range prepared.cleanup {
		pending.owner.acknowledgeNestedChildCleanup(pending.roots)
	}
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

func (e *Engine) processTreeKey(processID, parentID string) string {
	if process, ok := e.Process(processID); ok {
		return e.processTreeRootID(process)
	}
	if parentID != "" {
		if parent, ok := e.Process(parentID); ok {
			return e.processTreeRootID(parent)
		}
	}
	return processID
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
	engine   *Engine
	order    []string
	live     map[string]*Process
	reserved bool
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
	processes := make([]*Process, 0, len(tree.order))
	for _, id := range tree.order {
		if process := tree.live[id]; process != nil {
			processes = append(processes, process)
		}
	}
	if !tree.engine.processes.reserveProcesses(processes) {
		return fmt.Errorf("reserve process tree removal: %w", ErrProcessActive)
	}
	tree.reserved = true
	for _, id := range tree.order {
		process := tree.live[id]
		if process == nil {
			continue
		}
		if !process.state.removable() {
			tree.releaseClaims()
			return fmt.Errorf("claim process %q removal: %w", id, ErrProcessActive)
		}
	}
	return nil
}

func (tree *discoveredProcessTrees) releaseClaims() {
	if tree == nil {
		return
	}
	if tree.reserved {
		tree.engine.processes.releaseProcesses(tree.liveProcesses())
		tree.reserved = false
	}
}

func (tree *discoveredProcessTrees) release() error {
	if tree == nil || tree.engine == nil {
		return nil
	}
	processes := tree.liveProcesses()
	if !tree.engine.processes.unregisterReservedTree(processes) {
		tree.releaseClaims()
		return fmt.Errorf("release process tree registry ownership: %w", ErrProcessActive)
	}
	tree.reserved = false
	return nil
}

func (tree *discoveredProcessTrees) liveProcesses() []*Process {
	processes := make([]*Process, 0, len(tree.order))
	for _, id := range tree.order {
		if process := tree.live[id]; process != nil {
			processes = append(processes, process)
		}
	}
	return processes
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
