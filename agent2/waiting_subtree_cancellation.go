package agent2

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"
)

const waitingSubtreeParentPauseReason = "child Process cancellation requires explicit continuation"

var (
	// ErrInvalidWaitingSubtreeCancellationPlan reports a zero, malformed, or
	// foreign plan supplied to ApplyWaitingSubtreeCancellation.
	ErrInvalidWaitingSubtreeCancellationPlan = errors.New("agent: invalid waiting subtree cancellation plan")

	// ErrWaitingSubtreeCancellationUnavailable reports a target that is not a
	// non-root Waiting Process in the requested tree, or a tree state that cannot
	// represent the cancellation without violating its existing resource bounds.
	ErrWaitingSubtreeCancellationUnavailable = errors.New("agent: waiting subtree cancellation is unavailable")

	// ErrWaitingSubtreeCancellationPlanStale reports that the complete source
	// tree changed after its cancellation plan was produced. Apply makes no
	// Framework state change when it returns this error.
	ErrWaitingSubtreeCancellationPlanStale = errors.New("agent: waiting subtree cancellation plan is stale")
)

// WaitingSubtreeCancellationPlan is an immutable prospective transition for
// one Waiting non-root Process and every active descendant. It contains no
// live Engine lock or caller-owned resource.
type WaitingSubtreeCancellationPlan struct {
	engineIdentity     engineIdentity
	sourceRootID       ProcessID
	sourceDigest       Digest
	resultingSnapshot  TreeSnapshot
	canceledProcessIDs []ProcessID
	pausedProcessIDs   []ProcessID
}

// ResultingSnapshot returns the exact complete tree state produced by Apply.
func (plan WaitingSubtreeCancellationPlan) ResultingSnapshot() TreeSnapshot {
	return plan.resultingSnapshot
}

// CanceledProcessIDs returns Processes changed to Canceled, ordered from
// parent to child and then by ProcessID within one depth.
func (plan WaitingSubtreeCancellationPlan) CanceledProcessIDs() []ProcessID {
	return slices.Clone(plan.canceledProcessIDs)
}

// PausedProcessIDs returns parents newly paused before they can consume a
// child-completion Signal. A caller that elects to continue uses Process.Resume.
func (plan WaitingSubtreeCancellationPlan) PausedProcessIDs() []ProcessID {
	return slices.Clone(plan.pausedProcessIDs)
}

// Valid reports whether the plan contains one complete internally consistent
// prospective tree transition.
func (plan WaitingSubtreeCancellationPlan) Valid() bool {
	if !plan.engineIdentity.valid() || !plan.sourceRootID.Valid() || !plan.sourceDigest.Valid() ||
		!plan.resultingSnapshot.Valid() || plan.resultingSnapshot.RootID() != plan.sourceRootID ||
		len(plan.canceledProcessIDs) == 0 {
		return false
	}
	processes := make(map[ProcessID]Snapshot)
	for _, snapshot := range plan.resultingSnapshot.ProcessSnapshots() {
		processes[snapshot.ProcessID()] = snapshot
	}
	seen := make(map[ProcessID]struct{}, len(plan.canceledProcessIDs)+len(plan.pausedProcessIDs))
	var previousDepth uint32
	for index, id := range plan.canceledProcessIDs {
		snapshot, exists := processes[id]
		if !exists || snapshot.Status() != StatusCanceled {
			return false
		}
		if _, duplicate := seen[id]; duplicate {
			return false
		}
		depth := snapshot.Relation().Depth()
		if index > 0 && depth < previousDepth {
			return false
		}
		previousDepth = depth
		seen[id] = struct{}{}
	}
	for _, id := range plan.pausedProcessIDs {
		snapshot, exists := processes[id]
		if !exists || snapshot.Status() != StatusPaused {
			return false
		}
		if _, duplicate := seen[id]; duplicate {
			return false
		}
		seen[id] = struct{}{}
	}
	return true
}

// PlanWaitingSubtreeCancellation computes an immutable result at one complete
// Strategy-safe tree boundary without changing the live tree. targetID must
// identify a non-root Waiting Process in the tree rooted at rootID.
func (engine *Engine) PlanWaitingSubtreeCancellation(
	ctx context.Context,
	rootID ProcessID,
	targetID ProcessID,
	reason string,
) (WaitingSubtreeCancellationPlan, error) {
	if engine == nil || !rootID.Valid() || !targetID.Valid() {
		return WaitingSubtreeCancellationPlan{}, ErrInvalidProcessRelation
	}
	if err := validateTerminationReason(reason); err != nil {
		return WaitingSubtreeCancellationPlan{}, fmt.Errorf("%w: %w", ErrInvalidProcessControl, err)
	}
	ctx = contextOrBackground(ctx)
	engine.treeOperationMu.Lock()
	defer engine.treeOperationMu.Unlock()
	quiescence, err := engine.quiesceTree(ctx, rootID)
	if err != nil {
		return WaitingSubtreeCancellationPlan{}, err
	}
	defer quiescence.release()
	source, err := engine.captureQuiescedTree(ctx, rootID, quiescence.controllers)
	if err != nil {
		return WaitingSubtreeCancellationPlan{}, err
	}
	result, canceled, paused, err := projectWaitingSubtreeCancellation(
		source, targetID, reason, time.Now().Round(0).UTC(),
	)
	if err != nil {
		return WaitingSubtreeCancellationPlan{}, err
	}
	plan := WaitingSubtreeCancellationPlan{
		engineIdentity: engine.identity, sourceRootID: rootID,
		sourceDigest: digestBytes(source.JSON()), resultingSnapshot: result,
		canceledProcessIDs: canceled, pausedProcessIDs: paused,
	}
	if !plan.Valid() {
		return WaitingSubtreeCancellationPlan{}, ErrInvalidWaitingSubtreeCancellationPlan
	}
	return plan, nil
}

// ApplyWaitingSubtreeCancellation changes a live tree to the plan's exact
// Framework state. It accepts only a plan made by this Engine from the current
// complete tree. After the apply boundary is crossed, finalization completes
// even if ctx is canceled.
func (engine *Engine) ApplyWaitingSubtreeCancellation(
	ctx context.Context,
	plan WaitingSubtreeCancellationPlan,
) error {
	if engine == nil || !plan.Valid() || plan.engineIdentity != engine.identity {
		return ErrInvalidWaitingSubtreeCancellationPlan
	}
	ctx = contextOrBackground(ctx)
	engine.treeOperationMu.Lock()
	defer engine.treeOperationMu.Unlock()
	quiescence, err := engine.quiesceTree(ctx, plan.sourceRootID)
	if err != nil {
		return err
	}
	defer quiescence.release()
	source, err := engine.captureQuiescedTree(ctx, plan.sourceRootID, quiescence.controllers)
	if err != nil {
		return err
	}
	if digestBytes(source.JSON()) != plan.sourceDigest {
		return ErrWaitingSubtreeCancellationPlanStale
	}
	applyGate := make(chan struct{})
	plannedStates, err := newPlannedProcessStates(source, plan, applyGate)
	if err != nil {
		return err
	}
	registrations, err := childWaitRegistrationsFromSnapshot(plan.resultingSnapshot)
	if err != nil {
		return err
	}
	for _, plannedState := range plannedStates {
		controller := controllerByID(quiescence.controllers, plannedState.processID)
		if controller == nil {
			return ErrWaitingSubtreeCancellationPlanStale
		}
		response, requestErr := (&Process{controller: controller}).request(ctx, processCommand{
			kind: commandStagePlannedProcessState, plannedState: plannedState,
		})
		if requestErr != nil {
			return requestErr
		}
		if !response.accepted {
			return ErrEngineQuiescenceUnavailable
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	engine.replaceTreeChildWaits(plan.sourceRootID, registrations)
	close(applyGate)
	for _, plannedState := range plannedStates {
		<-plannedState.applied
	}
	quiescence.release()
	for _, id := range plan.canceledProcessIDs {
		controller := controllerByID(quiescence.controllers, id)
		if controller != nil {
			_ = waitTreeSettled(context.Background(), controller)
		}
	}
	return nil
}
