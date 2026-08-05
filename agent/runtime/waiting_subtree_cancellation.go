package runtime

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/core/chat"
)

const canceledChildToolResult = "error: delegated child canceled"

// WaitingSubtreeCancellationPlan is an immutable description of one validated
// process-tree transition. Planning observes one stable tree and releases it
// before returning; the value owns no live process, lock, or external resource.
// Applying the plan later succeeds only while that observed execution state is
// still current.
type WaitingSubtreeCancellationPlan struct {
	engine          *Engine
	rootProcessID   string
	targetProcessID string
	source          core.ProcessSnapshotTree
	result          core.ProcessSnapshotTree
	pending         []PendingSuspension
	canceled        []string
}

// PlanWaitingSubtreeCancellation computes the framework state transition for
// canceling a waiting non-root process. It performs no external I/O, publishes
// no event, mutates no live process, and retains no runtime ownership.
// rootProcessID must name the complete tree root; targetProcessID must name a
// waiting descendant in that tree.
func (e *Engine) PlanWaitingSubtreeCancellation(
	ctx context.Context,
	rootProcessID string,
	targetProcessID string,
) (*WaitingSubtreeCancellationPlan, error) {
	if e == nil {
		return nil, errors.New("runtime.Engine.PlanWaitingSubtreeCancellation: nil Engine")
	}
	if targetProcessID == rootProcessID {
		return nil, errors.New("runtime.Engine.PlanWaitingSubtreeCancellation: target must be a non-root process")
	}
	captured, err := e.SnapshotTree(ctx, rootProcessID)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.PlanWaitingSubtreeCancellation: %w", err)
	}
	transformed, canceled, _, err := cancelWaitingSnapshotSubtree(captured, targetProcessID)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.PlanWaitingSubtreeCancellation: %w", err)
	}
	pending, err := collectPendingSuspensions(transformed)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.PlanWaitingSubtreeCancellation: collect pending suspensions: %w", err)
	}
	return &WaitingSubtreeCancellationPlan{
		engine:          e,
		rootProcessID:   rootProcessID,
		targetProcessID: targetProcessID,
		source:          cloneProcessSnapshotTree(captured),
		result:          cloneProcessSnapshotTree(transformed),
		pending:         clonePendingSuspensions(pending),
		canceled:        slices.Clone(canceled),
	}, nil
}

// ResultingTree returns the ownership-isolated process tree produced by the
// planned transition.
func (p *WaitingSubtreeCancellationPlan) ResultingTree() core.ProcessSnapshotTree {
	if p == nil {
		return core.ProcessSnapshotTree{}
	}
	return cloneProcessSnapshotTree(p.result)
}

// PendingSuspensions returns the surviving external-input boundaries after the
// planned cancellation, in runtime execution order.
func (p *WaitingSubtreeCancellationPlan) PendingSuspensions() []PendingSuspension {
	if p == nil {
		return nil
	}
	return clonePendingSuspensions(p.pending)
}

// CanceledProcessIDs returns the exact target subtree in deterministic
// parent-before-child snapshot order.
func (p *WaitingSubtreeCancellationPlan) CanceledProcessIDs() []string {
	if p == nil {
		return nil
	}
	return slices.Clone(p.canceled)
}

// ApplyWaitingSubtreeCancellation applies one previously planned framework
// transition. It fails without mutation when the tree has changed since the
// plan was produced. The method coordinates only in-memory execution state; it
// knows nothing about persistence, transactions, or application workflow.
func (e *Engine) ApplyWaitingSubtreeCancellation(
	ctx context.Context,
	plan *WaitingSubtreeCancellationPlan,
) error {
	if e == nil {
		return errors.New("runtime.Engine.ApplyWaitingSubtreeCancellation: nil Engine")
	}
	if plan == nil {
		return errors.New("runtime.Engine.ApplyWaitingSubtreeCancellation: nil plan")
	}
	if plan.engine != e {
		return errors.New("runtime.Engine.ApplyWaitingSubtreeCancellation: plan belongs to a different Engine")
	}
	root, ok := e.processes.get(plan.rootProcessID)
	if !ok {
		return processNotFoundError("apply waiting subtree cancellation", plan.rootProcessID)
	}
	if root.ParentID() != "" || root.depth != 0 {
		return fmt.Errorf(
			"runtime.Engine.ApplyWaitingSubtreeCancellation: process %q is not a process-tree root",
			plan.rootProcessID,
		)
	}

	ctx = normalizeContext(ctx)
	releaseMutation, err := e.processMutations.acquire(ctx, plan.rootProcessID)
	if err != nil {
		return fmt.Errorf("runtime.Engine.ApplyWaitingSubtreeCancellation: acquire process tree: %w", err)
	}
	releaseMutationOnFailure := true
	defer func() {
		if releaseMutationOnFailure {
			releaseMutation()
		}
	}()
	if !e.processes.available(root) {
		return processNotFoundError("apply waiting subtree cancellation", plan.rootProcessID)
	}

	claimed, err := e.claimSnapshotTree(root, make(map[string]struct{}))
	if err != nil {
		return fmt.Errorf("runtime.Engine.ApplyWaitingSubtreeCancellation: %w", err)
	}
	releaseClaimsOnFailure := true
	defer func() {
		if releaseClaimsOnFailure {
			releaseSnapshotTree(claimed)
		}
	}()
	var snapshots []core.ProcessSnapshot
	if err := captureSnapshotTree(claimed, &snapshots); err != nil {
		return fmt.Errorf("runtime.Engine.ApplyWaitingSubtreeCancellation: capture: %w", err)
	}
	current := core.ProcessSnapshotTree{RootID: plan.rootProcessID, Snapshots: snapshots}
	if !reflect.DeepEqual(current, plan.source) {
		return fmt.Errorf(
			"runtime.Engine.ApplyWaitingSubtreeCancellation: process tree %q changed after planning: %w",
			plan.rootProcessID,
			interaction.ErrSuspensionStale,
		)
	}

	transformed, _, parentID, err := cancelWaitingSnapshotSubtree(current, plan.targetProcessID)
	if err != nil {
		return fmt.Errorf("runtime.Engine.ApplyWaitingSubtreeCancellation: %w", err)
	}
	if !reflect.DeepEqual(transformed, plan.result) {
		panic("runtime: waiting subtree cancellation plan produced a different result")
	}

	targetClaim := findClaimedSnapshotTree(claimed, plan.targetProcessID)
	if targetClaim == nil {
		return fmt.Errorf("%w: target process %q is absent from claimed tree", core.ErrInvalidSnapshot, plan.targetProcessID)
	}
	targetProcesses := flattenClaimedProcesses(targetClaim)
	if !e.processes.reserveProcesses(targetProcesses) {
		return fmt.Errorf("runtime.Engine.ApplyWaitingSubtreeCancellation: reserve target subtree: %w", ErrProcessActive)
	}
	releaseReservationOnFailure := true
	defer func() {
		if releaseReservationOnFailure {
			e.processes.releaseProcesses(targetProcesses)
		}
	}()
	parent, ok := e.processes.get(parentID)
	if !ok || !e.processes.available(parent) {
		return fmt.Errorf("%w: target parent process %q is unavailable", core.ErrInvalidSnapshot, parentID)
	}
	if !parent.budget.hasChild(targetClaim.process) {
		return fmt.Errorf("%w: target process %q is absent from parent budget ownership", core.ErrInvalidSnapshot, plan.targetProcessID)
	}
	parentSnapshot, ok := snapshotInTree(transformed, parentID)
	if !ok {
		return fmt.Errorf("%w: transformed parent process %q is missing", core.ErrInvalidSnapshot, parentID)
	}

	byID := make(map[string]core.ProcessSnapshot, len(transformed.Snapshots))
	for _, snapshot := range transformed.Snapshots {
		byID[snapshot.ID] = snapshot
	}
	canceledSet := make(map[string]struct{}, len(targetProcesses))
	for _, process := range targetProcesses {
		canceledSet[process.ID()] = struct{}{}
	}
	for _, process := range flattenClaimedProcesses(claimed) {
		if _, canceled := canceledSet[process.ID()]; canceled {
			continue
		}
		snapshot, ok := byID[process.ID()]
		if !ok {
			panic("runtime: waiting subtree cancellation lost a surviving snapshot")
		}
		if snapshot.Status == core.StatusWaiting {
			if !process.state.replaceClaimedSuspension(snapshot.Suspension) {
				panic("runtime: waiting subtree cancellation lost checkpoint ownership")
			}
			checkpoint, checkpointErr := nestedChildrenFromSuspension(snapshot.Suspension)
			if checkpointErr != nil {
				panic(fmt.Sprintf("runtime: waiting subtree cancellation contains invalid suspension: %v", checkpointErr))
			}
			process.commitNestedSuspension(checkpoint)
		}
	}
	parent.budget.replaceRetiredChildUsage(parentSnapshot.RetiredChildUsage)

	_, killed := e.killSubtreeOwned(targetClaim.process)
	releaseClaimedProcesses(targetProcesses)
	if !e.processes.unregisterReservedTree(targetProcesses) {
		panic("runtime: waiting subtree cancellation target changed while applying")
	}
	if !parent.budget.removeChild(targetClaim.process) {
		panic("runtime: waiting subtree cancellation lost parent budget ownership")
	}
	releaseProcessDeployments(targetProcesses)
	releaseSurvivingClaims(claimed, canceledSet)
	releaseMutation()
	releaseReservationOnFailure = false
	releaseClaimsOnFailure = false
	releaseMutationOnFailure = false

	publishKilledProcesses(
		normalizeContext(ctx),
		killed,
		"waiting delegated subtree canceled",
	)
	return nil
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
			InteractionID:  envelope.InteractionID,
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
