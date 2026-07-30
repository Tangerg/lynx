package runtime

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/core/chat"
)

const canceledChildToolResult = "error: delegated child canceled"

// PreparedWaitingSubtreeCancellation owns one stable process tree while its
// caller coordinates the replacement with external state. Prepare performs
// every fallible runtime check and computes the replacement snapshot without
// changing live execution state. The caller must call Commit after accepting
// the replacement, or Abort on every failure path.
//
// Holding a prepared value intentionally blocks Run, Resume, Kill, Snapshot,
// Restore, and removal mutations for the same tree. Values must not be copied.
type PreparedWaitingSubtreeCancellation struct {
	mu sync.Mutex

	engine          *Engine
	root            *claimedSnapshotTree
	releaseMutation func()
	target          *claimedSnapshotTree
	targetProcesses []*Process
	parent          *Process

	tree          core.ProcessSnapshotTree
	pending       []PendingSuspension
	canceled      []string
	parentRetired core.Usage
	settlement    preparedCancellationSettlement
}

type preparedCancellationSettlement uint8

const (
	preparedCancellationOpen preparedCancellationSettlement = iota
	preparedCancellationCommitted
	preparedCancellationAborted
)

// PrepareWaitingSubtreeCancellation freezes one complete idle tree and builds
// the exact checkpoint replacement for canceling a waiting non-root process.
// It performs no external I/O, publishes no event, and does not mutate live
// process state. rootProcessID must name the complete tree root;
// targetProcessID must name a waiting descendant in that tree.
func (e *Engine) PrepareWaitingSubtreeCancellation(
	ctx context.Context,
	rootProcessID string,
	targetProcessID string,
) (*PreparedWaitingSubtreeCancellation, error) {
	if e == nil {
		return nil, errors.New("runtime.Engine.PrepareWaitingSubtreeCancellation: nil Engine")
	}
	root, ok := e.processes.get(rootProcessID)
	if !ok {
		return nil, processNotFoundError("prepare waiting subtree cancellation", rootProcessID)
	}
	if root.ParentID() != "" || root.depth != 0 {
		return nil, fmt.Errorf(
			"runtime.Engine.PrepareWaitingSubtreeCancellation: process %q is not a process-tree root",
			rootProcessID,
		)
	}
	if targetProcessID == rootProcessID {
		return nil, errors.New("runtime.Engine.PrepareWaitingSubtreeCancellation: target must be a non-root process")
	}

	ctx = normalizeContext(ctx)
	releaseMutation, err := e.processMutations.acquire(ctx, rootProcessID)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.PrepareWaitingSubtreeCancellation: acquire process tree: %w", err)
	}
	releaseOnFailure := true
	defer func() {
		if releaseOnFailure {
			releaseMutation()
		}
	}()

	if !e.processes.available(root) {
		return nil, processNotFoundError("prepare waiting subtree cancellation", rootProcessID)
	}
	target, ok := e.processes.get(targetProcessID)
	if !ok || !e.processes.available(target) || e.processTreeRootID(target) != rootProcessID {
		return nil, processNotFoundError("prepare waiting subtree cancellation target", targetProcessID)
	}

	claimed, err := e.claimSnapshotTree(root, make(map[string]struct{}))
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.PrepareWaitingSubtreeCancellation: %w", err)
	}
	releaseClaimsOnFailure := true
	defer func() {
		if releaseClaimsOnFailure {
			releaseSnapshotTree(claimed)
		}
	}()

	var snapshots []core.ProcessSnapshot
	if err := captureSnapshotTree(claimed, &snapshots); err != nil {
		return nil, fmt.Errorf("runtime.Engine.PrepareWaitingSubtreeCancellation: capture: %w", err)
	}
	captured := core.ProcessSnapshotTree{RootID: rootProcessID, Snapshots: snapshots}
	transformed, canceled, parentID, err := cancelWaitingSnapshotSubtree(captured, targetProcessID)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.PrepareWaitingSubtreeCancellation: %w", err)
	}
	pending, err := collectPendingSuspensions(transformed)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.PrepareWaitingSubtreeCancellation: collect pending suspensions: %w", err)
	}

	targetClaim := findClaimedSnapshotTree(claimed, targetProcessID)
	if targetClaim == nil {
		return nil, fmt.Errorf("%w: target process %q is absent from claimed tree", core.ErrInvalidSnapshot, targetProcessID)
	}
	targetProcesses := flattenClaimedProcesses(targetClaim)
	if !e.processes.reserveProcesses(targetProcesses) {
		return nil, fmt.Errorf("runtime.Engine.PrepareWaitingSubtreeCancellation: reserve target subtree: %w", ErrProcessActive)
	}
	releaseReservationOnFailure := true
	defer func() {
		if releaseReservationOnFailure {
			e.processes.releaseProcesses(targetProcesses)
		}
	}()

	parent, ok := e.processes.get(parentID)
	if !ok || !e.processes.available(parent) {
		return nil, fmt.Errorf("%w: target parent process %q is unavailable", core.ErrInvalidSnapshot, parentID)
	}
	if !parent.budget.hasChild(target) {
		return nil, fmt.Errorf("%w: target process %q is absent from parent budget ownership", core.ErrInvalidSnapshot, targetProcessID)
	}
	parentSnapshot, ok := snapshotInTree(transformed, parentID)
	if !ok {
		return nil, fmt.Errorf("%w: transformed parent process %q is missing", core.ErrInvalidSnapshot, parentID)
	}

	prepared := &PreparedWaitingSubtreeCancellation{
		engine:          e,
		root:            claimed,
		releaseMutation: releaseMutation,
		target:          targetClaim,
		targetProcesses: targetProcesses,
		parent:          parent,
		tree:            transformed,
		pending:         clonePendingSuspensions(pending),
		canceled:        slices.Clone(canceled),
		parentRetired:   parentSnapshot.RetiredChildUsage,
	}
	releaseReservationOnFailure = false
	releaseClaimsOnFailure = false
	releaseOnFailure = false
	return prepared, nil
}

// SnapshotTree returns the ownership-isolated replacement for the caller to
// coordinate with its external state transition.
func (p *PreparedWaitingSubtreeCancellation) SnapshotTree() core.ProcessSnapshotTree {
	if p == nil {
		return core.ProcessSnapshotTree{}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return cloneProcessSnapshotTree(p.tree)
}

// PendingSuspensions returns the surviving external-input boundaries after the
// prepared cancellation, in runtime execution order.
func (p *PreparedWaitingSubtreeCancellation) PendingSuspensions() []PendingSuspension {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return clonePendingSuspensions(p.pending)
}

// CanceledProcessIDs returns the exact target subtree in deterministic
// parent-before-child snapshot order.
func (p *PreparedWaitingSubtreeCancellation) CanceledProcessIDs() []string {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.canceled)
}

// Commit applies the already-validated in-memory checkpoint replacement,
// detaches the canceled subtree, releases tree ownership, and then publishes
// ProcessKilled for processes that became terminal. Calling Commit again is
// idempotent. Calling it after Abort fails because a replacement must not be
// accepted after runtime ownership was relinquished.
func (p *PreparedWaitingSubtreeCancellation) Commit(ctx context.Context) error {
	if p == nil {
		return errors.New("runtime.PreparedWaitingSubtreeCancellation.Commit: nil receiver")
	}
	p.mu.Lock()
	switch p.settlement {
	case preparedCancellationCommitted:
		p.mu.Unlock()
		return nil
	case preparedCancellationAborted:
		p.mu.Unlock()
		return errors.New("runtime.PreparedWaitingSubtreeCancellation.Commit: cancellation was aborted")
	}

	byID := make(map[string]core.ProcessSnapshot, len(p.tree.Snapshots))
	for _, snapshot := range p.tree.Snapshots {
		byID[snapshot.ID] = snapshot
	}
	canceledSet := make(map[string]struct{}, len(p.targetProcesses))
	for _, process := range p.targetProcesses {
		canceledSet[process.ID()] = struct{}{}
	}
	for _, process := range flattenClaimedProcesses(p.root) {
		if _, canceled := canceledSet[process.ID()]; canceled {
			continue
		}
		snapshot, ok := byID[process.ID()]
		if !ok {
			panic("runtime: prepared waiting subtree cancellation lost a surviving snapshot")
		}
		if snapshot.Status == core.StatusWaiting {
			if !process.state.replaceClaimedSuspension(snapshot.Suspension) {
				panic("runtime: prepared waiting subtree cancellation lost checkpoint ownership")
			}
			checkpoint, err := nestedChildrenFromSuspension(snapshot.Suspension)
			if err != nil {
				panic(fmt.Sprintf("runtime: prepared waiting subtree cancellation contains invalid suspension: %v", err))
			}
			process.commitNestedSuspension(checkpoint)
		}
	}
	p.parent.budget.replaceRetiredChildUsage(p.parentRetired)

	_, killed := p.engine.killSubtreeOwned(p.target.process)
	releaseClaimedProcesses(p.targetProcesses)
	if !p.engine.processes.unregisterReservedTree(p.targetProcesses) {
		panic("runtime: prepared waiting subtree cancellation target changed before commit")
	}
	if !p.parent.budget.removeChild(p.target.process) {
		panic("runtime: prepared waiting subtree cancellation lost parent budget ownership")
	}
	releaseProcessDeployments(p.targetProcesses)
	releaseSurvivingClaims(p.root, canceledSet)
	p.releaseMutation()
	p.settlement = preparedCancellationCommitted
	p.mu.Unlock()

	publishKilledProcesses(
		normalizeContext(ctx),
		killed,
		"waiting delegated subtree cancellation committed",
	)
	return nil
}

// Abort releases all runtime ownership without changing live state. It is safe
// to defer Abort immediately after Prepare; Abort after Commit is a no-op.
func (p *PreparedWaitingSubtreeCancellation) Abort() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.settlement != preparedCancellationOpen {
		return
	}
	p.engine.processes.releaseProcesses(p.targetProcesses)
	releaseSnapshotTree(p.root)
	p.releaseMutation()
	p.settlement = preparedCancellationAborted
}

func cancelWaitingSnapshotSubtree(
	tree core.ProcessSnapshotTree,
	targetProcessID string,
) (core.ProcessSnapshotTree, []string, string, error) {
	if err := tree.Validate(); err != nil {
		return core.ProcessSnapshotTree{}, nil, "", err
	}
	if err := validateNestedSnapshotRelations(tree); err != nil {
		return core.ProcessSnapshotTree{}, nil, "", err
	}
	if targetProcessID == tree.RootID {
		return core.ProcessSnapshotTree{}, nil, "", errors.New("waiting subtree cancellation target must not be the tree root")
	}

	transformed := cloneProcessSnapshotTree(tree)
	indexByID := make(map[string]int, len(transformed.Snapshots))
	children := make(map[string][]string, len(transformed.Snapshots))
	for index, snapshot := range transformed.Snapshots {
		indexByID[snapshot.ID] = index
		children[snapshot.ParentID] = append(children[snapshot.ParentID], snapshot.ID)
	}
	targetIndex, ok := indexByID[targetProcessID]
	if !ok {
		return core.ProcessSnapshotTree{}, nil, "", processNotFoundError("cancel waiting snapshot subtree", targetProcessID)
	}
	target := transformed.Snapshots[targetIndex]
	if target.Status != core.StatusWaiting {
		return core.ProcessSnapshotTree{}, nil, "", fmt.Errorf(
			"%w: target process %q is %s, want waiting",
			interaction.ErrSuspensionStale,
			targetProcessID,
			target.Status,
		)
	}
	parentIndex, ok := indexByID[target.ParentID]
	if !ok {
		return core.ProcessSnapshotTree{}, nil, "", fmt.Errorf("%w: target parent process %q is missing", core.ErrInvalidSnapshot, target.ParentID)
	}

	var canceled []string
	var collectSubtree func(string)
	collectSubtree = func(processID string) {
		canceled = append(canceled, processID)
		for _, childID := range children[processID] {
			collectSubtree(childID)
		}
	}
	collectSubtree(targetProcessID)
	canceledSet := make(map[string]struct{}, len(canceled))
	var canceledUsage core.Usage
	for _, processID := range canceled {
		canceledSet[processID] = struct{}{}
		snapshot := transformed.Snapshots[indexByID[processID]]
		var err error
		canceledUsage, err = addUsage(canceledUsage, snapshot.OwnUsage)
		if err != nil {
			return core.ProcessSnapshotTree{}, nil, "", fmt.Errorf("%w: canceled subtree usage: %w", core.ErrInvalidSnapshot, err)
		}
		canceledUsage, err = addUsage(canceledUsage, snapshot.RetiredChildUsage)
		if err != nil {
			return core.ProcessSnapshotTree{}, nil, "", fmt.Errorf("%w: canceled subtree retired usage: %w", core.ErrInvalidSnapshot, err)
		}
	}

	parent := &transformed.Snapshots[parentIndex]
	rewritten, branchReady, err := settleCanceledChild(parent.Suspension, targetProcessID)
	if err != nil {
		return core.ProcessSnapshotTree{}, nil, "", fmt.Errorf("process %q checkpoint: %w", parent.ID, err)
	}
	parent.Suspension = rewritten
	parent.RetiredChildUsage, err = addUsage(parent.RetiredChildUsage, canceledUsage)
	if err != nil {
		return core.ProcessSnapshotTree{}, nil, "", fmt.Errorf("%w: absorb canceled subtree usage into parent %q: %w", core.ErrInvalidSnapshot, parent.ID, err)
	}

	currentID := parent.ID
	for branchReady && currentID != tree.RootID {
		current := transformed.Snapshots[indexByID[currentID]]
		ancestorIndex := indexByID[current.ParentID]
		ancestor := &transformed.Snapshots[ancestorIndex]
		ancestor.Suspension, branchReady, err = markNestedChildReady(ancestor.Suspension, currentID)
		if err != nil {
			return core.ProcessSnapshotTree{}, nil, "", fmt.Errorf("process %q checkpoint: %w", ancestor.ID, err)
		}
		currentID = ancestor.ID
	}

	transformed.Snapshots = slices.DeleteFunc(transformed.Snapshots, func(snapshot core.ProcessSnapshot) bool {
		_, remove := canceledSet[snapshot.ID]
		return remove
	})
	if err := transformed.Validate(); err != nil {
		return core.ProcessSnapshotTree{}, nil, "", err
	}
	if err := validateNestedSnapshotRelations(transformed); err != nil {
		return core.ProcessSnapshotTree{}, nil, "", err
	}
	return transformed, canceled, target.ParentID, nil
}

func settleCanceledChild(
	suspension *interaction.Suspension,
	childProcessID string,
) (*interaction.Suspension, bool, error) {
	if suspension == nil {
		return nil, false, fmt.Errorf("%w: parent has no suspension", core.ErrInvalidSnapshot)
	}
	envelope, err := decodeSuspensionCheckpoint(suspension.FrameworkState)
	if err != nil {
		return nil, false, err
	}
	if envelope == nil {
		return nil, false, fmt.Errorf("%w: parent suspension has no nested child checkpoint", core.ErrInvalidSnapshot)
	}
	rewritten := suspension.Clone()

	switch envelope.Kind {
	case suspensionCheckpointNestedChild:
		relation := envelope.NestedChildren[0]
		if relation.ChildID != childProcessID {
			return nil, false, fmt.Errorf("%w: parent checkpoint does not own child %q", core.ErrInvalidSnapshot, childProcessID)
		}
		rewritten.Response = nil
		rewritten.FrameworkState, err = encodeSuspensionCheckpoint(suspensionCheckpoint{
			SchemaVersion: suspensionCheckpointSchemaVersion,
			Kind:          suspensionCheckpointChildCanceled,
			CanceledChild: relation,
		})
		return rewritten, true, err

	case suspensionCheckpointInteraction:
		active, err := validateCheckpointNestedChildren(envelope.Checkpoint, envelope.NestedChildren)
		if err != nil {
			return nil, false, err
		}
		var target *nestedChildRelation
		for _, relation := range envelope.NestedChildren {
			if relation.ChildID == childProcessID {
				target = relation
				break
			}
		}
		if target == nil {
			return nil, false, fmt.Errorf("%w: parent checkpoint does not own child %q", core.ErrInvalidSnapshot, childProcessID)
		}
		completed, err := envelope.Checkpoint.CompletePausedCall(target.ToolCallID, chat.ToolResult{
			ID:      target.ToolCallID,
			Name:    target.ToolName,
			Result:  canceledChildToolResult,
			IsError: true,
		})
		if err != nil {
			return nil, false, fmt.Errorf("settle canceled child tool call %q: %w", target.ToolCallID, err)
		}
		relations := slices.DeleteFunc(cloneNestedChildRelations(envelope.NestedChildren), func(relation *nestedChildRelation) bool {
			return relation.ChildID == childProcessID
		})
		branchReady := active != nil && active.ChildID == childProcessID
		ready := envelope.Ready && !branchReady
		if branchReady {
			rewritten.Response = nil
		}
		rewritten.FrameworkState, err = encodeSuspensionCheckpoint(suspensionCheckpoint{
			SchemaVersion:  suspensionCheckpointSchemaVersion,
			Kind:           suspensionCheckpointInteraction,
			Owner:          envelope.Owner,
			Deployment:     envelope.Deployment,
			Checkpoint:     completed,
			NestedChildren: relations,
			Ready:          ready,
		})
		return rewritten, branchReady, err

	default:
		return nil, false, fmt.Errorf("%w: checkpoint kind %q has no live child", core.ErrInvalidSnapshot, envelope.Kind)
	}
}

func markNestedChildReady(
	suspension *interaction.Suspension,
	childProcessID string,
) (*interaction.Suspension, bool, error) {
	if suspension == nil {
		return nil, false, fmt.Errorf("%w: ancestor has no suspension", core.ErrInvalidSnapshot)
	}
	envelope, err := decodeSuspensionCheckpoint(suspension.FrameworkState)
	if err != nil {
		return nil, false, err
	}
	if envelope == nil {
		return nil, false, fmt.Errorf("%w: ancestor suspension has no nested child checkpoint", core.ErrInvalidSnapshot)
	}
	checkpoint, err := nestedChildrenFromSuspension(suspension)
	if err != nil {
		return nil, false, err
	}
	var relation *nestedChildRelation
	for _, candidate := range checkpoint.relations {
		if candidate.ChildID == childProcessID {
			relation = candidate
			break
		}
	}
	if relation == nil {
		return nil, false, fmt.Errorf("%w: ancestor checkpoint does not own child %q", core.ErrInvalidSnapshot, childProcessID)
	}
	if checkpoint.active == nil || checkpoint.active.ChildID != childProcessID {
		return suspension.Clone(), false, nil
	}
	envelope.Ready = true
	rewritten := suspension.Clone()
	rewritten.Response = nil
	rewritten.FrameworkState, err = encodeSuspensionCheckpoint(*envelope)
	if err != nil {
		return nil, false, err
	}
	return rewritten, true, nil
}

func findClaimedSnapshotTree(tree *claimedSnapshotTree, processID string) *claimedSnapshotTree {
	if tree == nil || tree.process == nil {
		return nil
	}
	if tree.process.ID() == processID {
		return tree
	}
	for _, child := range tree.children {
		if found := findClaimedSnapshotTree(child, processID); found != nil {
			return found
		}
	}
	return nil
}

func flattenClaimedProcesses(tree *claimedSnapshotTree) []*Process {
	if tree == nil || tree.process == nil {
		return nil
	}
	processes := []*Process{tree.process}
	for _, child := range tree.children {
		processes = append(processes, flattenClaimedProcesses(child)...)
	}
	return processes
}

func releaseClaimedProcesses(processes []*Process) {
	for index := len(processes) - 1; index >= 0; index-- {
		processes[index].state.releaseCheckpoint()
	}
}

func releaseSurvivingClaims(tree *claimedSnapshotTree, canceled map[string]struct{}) {
	if tree == nil || tree.process == nil {
		return
	}
	for index := len(tree.children) - 1; index >= 0; index-- {
		releaseSurvivingClaims(tree.children[index], canceled)
	}
	if _, removed := canceled[tree.process.ID()]; !removed {
		tree.process.state.releaseCheckpoint()
	}
}

func snapshotInTree(tree core.ProcessSnapshotTree, processID string) (core.ProcessSnapshot, bool) {
	for _, snapshot := range tree.Snapshots {
		if snapshot.ID == processID {
			return snapshot, true
		}
	}
	return core.ProcessSnapshot{}, false
}
