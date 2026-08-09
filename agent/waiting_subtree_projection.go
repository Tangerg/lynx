package agent

import (
	"fmt"
	"slices"
	"time"
)

func projectWaitingSubtreeCancellation(
	source TreeSnapshot,
	targetID ProcessID,
	reason string,
	finishedAt time.Time,
) (TreeSnapshot, []ProcessID, []ProcessID, error) {
	projection, err := newWaitingSubtreeProjection(source, targetID, finishedAt)
	if err != nil {
		return TreeSnapshot{}, nil, nil, err
	}
	if err := projection.cancelActiveSubtree(reason); err != nil {
		return TreeSnapshot{}, nil, nil, err
	}
	if err := projection.reconcileChildWaits(); err != nil {
		return TreeSnapshot{}, nil, nil, err
	}
	return projection.result()
}

type waitingSubtreeProjection struct {
	tree        treeSnapshotWire
	processes   map[ProcessID]processSnapshotWire
	indexes     map[ProcessID]int
	targetID    ProcessID
	finishedAt  time.Time
	canceled    []ProcessID
	canceledSet map[ProcessID]struct{}
	pausedSet   map[ProcessID]struct{}
}

func newWaitingSubtreeProjection(
	source TreeSnapshot,
	targetID ProcessID,
	finishedAt time.Time,
) (*waitingSubtreeProjection, error) {
	wire, err := source.wire()
	if err != nil {
		return nil, err
	}
	processes, indexes, err := processSnapshotWires(wire.ProcessSnapshots)
	if err != nil {
		return nil, err
	}
	target, exists := processes[targetID]
	if !exists || targetID == wire.RootID || target.Status != StatusWaiting ||
		target.Prepared != nil || !emptyPendingControl(target.PendingControl) {
		return nil, ErrWaitingSubtreeCancellationUnavailable
	}
	return &waitingSubtreeProjection{
		tree: wire, processes: processes, indexes: indexes, targetID: targetID,
		finishedAt: finishedAt, canceledSet: make(map[ProcessID]struct{}),
		pausedSet: make(map[ProcessID]struct{}),
	}, nil
}

func (projection *waitingSubtreeProjection) cancelActiveSubtree(reason string) error {
	for _, snapshot := range projection.tree.ProcessSnapshots {
		candidate := projection.processes[snapshot.ProcessID()]
		if !candidate.Status.Terminal() && projection.belongsToTarget(candidate.ProcessID) {
			if candidate.Prepared != nil || !emptyPendingControl(candidate.PendingControl) {
				return ErrWaitingSubtreeCancellationUnavailable
			}
			projection.canceled = append(projection.canceled, candidate.ProcessID)
		}
	}
	if len(projection.canceled) == 0 {
		return ErrWaitingSubtreeCancellationUnavailable
	}
	for _, id := range projection.canceled {
		projection.canceledSet[id] = struct{}{}
		intentOwner := cancellationOwnerParent
		terminationReason := "parent Process reached terminal status canceled"
		if id == projection.targetID {
			intentOwner = cancellationOwnerHost
			terminationReason = reason
		}
		intent, err := newCancellationIntent(intentOwner, terminationReason)
		if err != nil {
			return err
		}
		termination, err := resolveTermination(terminationFacts{cancellation: intent})
		if err != nil {
			return err
		}
		if err := projection.cancelProcess(id, termination); err != nil {
			return err
		}
	}
	return nil
}

func (projection *waitingSubtreeProjection) cancelProcess(
	processID ProcessID,
	termination Termination,
) error {
	process := projection.processes[processID]
	mailbox, err := restoreSignalMailbox(process.Mailbox)
	if err != nil || process.ProcessEventSequence == ^uint64(0) {
		return ErrWaitingSubtreeCancellationUnavailable
	}
	mailbox.closeAllWaits()
	finishedAt := projection.finishedAt
	process.Status = StatusCanceled
	process.FinishedAt = &finishedAt
	process.ProcessEventSequence++
	process.Mailbox = mailbox.snapshot()
	process.Prepared = nil
	process.CurrentWaitID = nil
	process.PauseReason = ""
	process.PendingControl = pendingControlWire{}
	process.Output = nil
	process.Termination = &termination
	return projection.replaceProcess(process)
}

func (projection *waitingSubtreeProjection) reconcileChildWaits() error {
	surviving := make([]childWaitSnapshotWire, 0, len(projection.tree.ChildWaits))
	for _, wait := range projection.tree.ChildWaits {
		if _, canceledParent := projection.canceledSet[wait.ParentProcessID]; canceledParent {
			continue
		}
		surviving = append(surviving, wait)
		if !projection.childWaitTouchesCanceledProcess(wait.Spec) {
			continue
		}
		if err := projection.deliverBoundaryCompletion(wait); err != nil {
			return err
		}
	}
	projection.tree.ChildWaits = surviving
	return nil
}

func (projection *waitingSubtreeProjection) deliverBoundaryCompletion(
	wait childWaitSnapshotWire,
) error {
	spec, err := wait.Spec.value()
	if err != nil {
		return err
	}
	outcomes, ready, err := projection.childOutcomes(spec)
	if err != nil {
		return err
	}
	if !ready {
		return nil
	}
	signal, err := encodeChildrenCompletedAt(wait.WaitID, spec.Key, outcomes, projection.finishedAt)
	if err != nil {
		return err
	}
	parent := projection.processes[wait.ParentProcessID]
	if parent.Status.Terminal() || parent.Prepared != nil || !emptyPendingControl(parent.PendingControl) {
		return ErrWaitingSubtreeCancellationUnavailable
	}
	mailbox, err := restoreSignalMailbox(parent.Mailbox)
	if err != nil {
		return err
	}
	if !mailbox.contains(signal.ID()) {
		if parent.Usage.AcceptedSignals >= parent.Limits.MaxSignals ||
			mailbox.pendingCount() >= parent.Limits.MaxPendingSignals ||
			parent.Usage.AcceptedSignals+1+parent.ReservedBudget.Signals > parent.Budget.Signals {
			return fmt.Errorf(
				"%w: %w", ErrWaitingSubtreeCancellationUnavailable, ErrResourceLimitExceeded,
			)
		}
		accepted, err := mailbox.enqueueChildCompletion(parent.Status, signal)
		if err != nil {
			return err
		}
		if !accepted || parent.ProcessEventSequence == ^uint64(0) {
			return ErrWaitingSubtreeCancellationUnavailable
		}
		parent.Usage.AcceptedSignals++
		parent.ProcessEventSequence++
	}
	if parent.Status == StatusRunning || parent.Status == StatusWaiting {
		if parent.ProcessEventSequence == ^uint64(0) {
			return ErrWaitingSubtreeCancellationUnavailable
		}
		parent.Status = StatusPaused
		parent.CurrentWaitID = nil
		parent.PauseReason = waitingSubtreeParentPauseReason
		parent.PendingControl = pendingControlWire{}
		parent.ProcessEventSequence++
		projection.pausedSet[parent.ProcessID] = struct{}{}
	}
	parent.Mailbox = mailbox.snapshot()
	return projection.replaceProcess(parent)
}

func (projection *waitingSubtreeProjection) result() (
	TreeSnapshot,
	[]ProcessID,
	[]ProcessID,
	error,
) {
	result, err := newTreeSnapshot(projection.tree)
	if err != nil {
		return TreeSnapshot{}, nil, nil, err
	}
	paused := make([]ProcessID, 0, len(projection.pausedSet))
	for _, snapshot := range result.ProcessSnapshots() {
		if _, exists := projection.pausedSet[snapshot.ProcessID()]; exists {
			paused = append(paused, snapshot.ProcessID())
		}
	}
	return result, slices.Clone(projection.canceled), paused, nil
}

func processSnapshotWires(
	snapshots []Snapshot,
) (map[ProcessID]processSnapshotWire, map[ProcessID]int, error) {
	wires := make(map[ProcessID]processSnapshotWire, len(snapshots))
	indexes := make(map[ProcessID]int, len(snapshots))
	for index, snapshot := range snapshots {
		wire, err := snapshot.wire()
		if err != nil {
			return nil, nil, err
		}
		wires[wire.ProcessID] = wire
		indexes[wire.ProcessID] = index
	}
	return wires, indexes, nil
}

func (projection *waitingSubtreeProjection) replaceProcess(process processSnapshotWire) error {
	snapshot, err := newSnapshot(process)
	if err != nil {
		return err
	}
	projection.processes[process.ProcessID] = process
	projection.tree.ProcessSnapshots[projection.indexes[process.ProcessID]] = snapshot
	return nil
}

func (projection *waitingSubtreeProjection) belongsToTarget(processID ProcessID) bool {
	for processID.Valid() {
		if processID == projection.targetID {
			return true
		}
		process, exists := projection.processes[processID]
		if !exists {
			return false
		}
		relation, err := processRelationFromWire(processID, process.Relation)
		if err != nil {
			return false
		}
		parentID, child := relation.ParentID()
		if !child {
			return false
		}
		processID = parentID
	}
	return false
}

func (projection *waitingSubtreeProjection) childWaitTouchesCanceledProcess(
	spec childWaitSpecWire,
) bool {
	for _, childID := range spec.Children {
		if _, exists := projection.canceledSet[childID]; exists {
			return true
		}
	}
	return false
}

func (projection *waitingSubtreeProjection) childOutcomes(
	spec ChildWaitSpec,
) ([]ChildOutcome, bool, error) {
	outcomes := make([]ChildOutcome, 0, len(spec.Children))
	for _, childID := range spec.Children {
		process, exists := projection.processes[childID]
		if !exists {
			return nil, false, ErrInvalidChildWait
		}
		if !process.Status.Terminal() {
			continue
		}
		relation, err := processRelationFromWire(process.ProcessID, process.Relation)
		if err != nil {
			return nil, false, err
		}
		key, child := relation.ChildKey()
		if !child {
			return nil, false, ErrInvalidChildWait
		}
		result := resultFromProcessSnapshotWire(process)
		if !result.Valid() {
			return nil, false, ErrInvalidChildWait
		}
		outcomes = append(outcomes, ChildOutcome{key: key, result: result})
	}
	required, err := spec.Condition.required(len(spec.Children))
	return outcomes, err == nil && uint32(len(outcomes)) >= required, err
}

func resultFromProcessSnapshotWire(process processSnapshotWire) Result {
	var output Output
	if process.Output != nil {
		output = *process.Output
	}
	var termination Termination
	if process.Termination != nil {
		termination = *process.Termination
	}
	var finishedAt time.Time
	if process.FinishedAt != nil {
		finishedAt = *process.FinishedAt
	}
	return Result{
		processID: process.ProcessID, startedAt: process.StartedAt, finishedAt: finishedAt,
		output: output, termination: termination, usage: process.Usage,
	}
}
