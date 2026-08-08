package agent2

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"
)

const waitingSubtreeParentPauseReason = "child Process cancellation requires explicit continuation"

var (
	// ErrInvalidPreparedWaitingSubtreeCancellation reports a nil or internally
	// invalid prepared cancellation capability.
	ErrInvalidPreparedWaitingSubtreeCancellation = errors.New("agent: invalid prepared waiting subtree cancellation")

	// ErrPreparedWaitingSubtreeCancellationResolved reports a second Apply or
	// Discard call after the prepared cancellation was already resolved.
	ErrPreparedWaitingSubtreeCancellationResolved = errors.New("agent: prepared waiting subtree cancellation is resolved")

	// ErrWaitingSubtreeCancellationUnavailable reports a target that is not a
	// non-root Waiting Process in the requested tree, or a tree state that cannot
	// represent the cancellation without violating its existing resource bounds.
	ErrWaitingSubtreeCancellationUnavailable = errors.New("agent: waiting subtree cancellation is unavailable")
)

// PreparedWaitingSubtreeCancellation owns one frozen, Strategy-safe source
// tree and its exact prospective cancellation result. It must not be copied
// after first use. The caller must resolve it exactly once with Apply or
// Discard; until then that source tree remains frozen. Other root trees in the
// Engine remain independent.
type PreparedWaitingSubtreeCancellation struct {
	engine                 *Engine
	operation              *treeOperation
	quiescence             *treeQuiescence
	resultingSnapshot      TreeSnapshot
	canceledProcessIDs     []ProcessID
	pausedProcessIDs       []ProcessID
	preparedStateChanges   []*preparedProcessStateChange
	childWaitRegistrations []*childWaitRegistration
	applyGate              chan struct{}

	resolutionMu sync.Mutex
	resolved     bool
}

// ResultingSnapshot returns the exact complete tree state that Apply will
// install. The snapshot remains readable after Apply or Discard.
func (prepared *PreparedWaitingSubtreeCancellation) ResultingSnapshot() TreeSnapshot {
	if prepared == nil {
		return TreeSnapshot{}
	}
	return prepared.resultingSnapshot
}

// CanceledProcessIDs returns Processes projected as Canceled, ordered from
// parent to child and then by ProcessID within one depth.
func (prepared *PreparedWaitingSubtreeCancellation) CanceledProcessIDs() []ProcessID {
	if prepared == nil {
		return nil
	}
	return slices.Clone(prepared.canceledProcessIDs)
}

// PausedProcessIDs returns parents projected as Paused before they can consume
// a child-completion Signal. A caller that later continues uses Process.Resume.
func (prepared *PreparedWaitingSubtreeCancellation) PausedProcessIDs() []ProcessID {
	if prepared == nil {
		return nil
	}
	return slices.Clone(prepared.pausedProcessIDs)
}

// Apply commits the exact prepared Framework state and releases the frozen
// source tree. An error before the apply boundary discards the prepared change
// and leaves the tree unchanged; after the boundary, finalization is bounded
// and completes independently of ctx.
func (prepared *PreparedWaitingSubtreeCancellation) Apply(ctx context.Context) error {
	if prepared == nil {
		return ErrInvalidPreparedWaitingSubtreeCancellation
	}
	prepared.resolutionMu.Lock()
	defer prepared.resolutionMu.Unlock()
	if prepared.resolved {
		return ErrPreparedWaitingSubtreeCancellationResolved
	}
	if !prepared.valid() {
		prepared.resolveLocked()
		return ErrInvalidPreparedWaitingSubtreeCancellation
	}
	ctx = contextOrBackground(ctx)
	for _, change := range prepared.preparedStateChanges {
		controller := controllerByID(prepared.quiescence.controllers, change.processID)
		if controller == nil {
			prepared.resolveLocked()
			return ErrEngineQuiescenceUnavailable
		}
		response, err := (&Process{controller: controller}).request(ctx, processCommand{
			kind: commandStagePreparedProcessState, preparedStateChange: change,
		})
		if err != nil {
			prepared.resolveLocked()
			return err
		}
		if !response.accepted {
			prepared.resolveLocked()
			return ErrEngineQuiescenceUnavailable
		}
	}
	if err := ctx.Err(); err != nil {
		prepared.resolveLocked()
		return err
	}
	prepared.engine.replaceTreeChildWaits(
		prepared.resultingSnapshot.RootID(), prepared.childWaitRegistrations,
	)
	close(prepared.applyGate)
	for _, change := range prepared.preparedStateChanges {
		<-change.applied
	}
	prepared.quiescence.release()
	for _, processID := range prepared.canceledProcessIDs {
		controller := controllerByID(prepared.quiescence.controllers, processID)
		if controller != nil {
			_ = waitTreeSettled(context.Background(), controller)
		}
	}
	prepared.resolveLocked()
	return nil
}

// Discard releases the frozen source tree without applying the prospective
// cancellation. It returns an error when Apply or Discard already resolved it.
func (prepared *PreparedWaitingSubtreeCancellation) Discard() error {
	if prepared == nil {
		return ErrInvalidPreparedWaitingSubtreeCancellation
	}
	prepared.resolutionMu.Lock()
	defer prepared.resolutionMu.Unlock()
	if prepared.resolved {
		return ErrPreparedWaitingSubtreeCancellationResolved
	}
	if !prepared.valid() {
		prepared.resolveLocked()
		return ErrInvalidPreparedWaitingSubtreeCancellation
	}
	prepared.resolveLocked()
	return nil
}

func (prepared *PreparedWaitingSubtreeCancellation) valid() bool {
	return prepared.engine != nil && prepared.operation != nil &&
		prepared.operation.engine == prepared.engine && prepared.quiescence != nil &&
		prepared.operation.rootID == prepared.resultingSnapshot.RootID() &&
		!prepared.quiescence.released && prepared.resultingSnapshot.Valid() &&
		len(prepared.canceledProcessIDs) > 0 &&
		len(prepared.preparedStateChanges) ==
			len(prepared.canceledProcessIDs)+len(prepared.pausedProcessIDs) &&
		prepared.applyGate != nil &&
		validWaitingSubtreeCancellationProjection(
			prepared.resultingSnapshot,
			prepared.canceledProcessIDs,
			prepared.pausedProcessIDs,
		)
}

func (prepared *PreparedWaitingSubtreeCancellation) resolveLocked() {
	prepared.quiescence.release()
	prepared.operation.release()
	prepared.engine = nil
	prepared.operation = nil
	prepared.quiescence = nil
	prepared.preparedStateChanges = nil
	prepared.childWaitRegistrations = nil
	prepared.applyGate = nil
	prepared.resolved = true
}

// PrepareWaitingSubtreeCancellation freezes one complete source tree and
// computes its exact cancellation result. targetID must identify a non-root
// Waiting Process in the tree rooted at rootID. The returned capability must be
// resolved exactly once with Apply or Discard.
func (engine *Engine) PrepareWaitingSubtreeCancellation(
	ctx context.Context,
	rootID ProcessID,
	targetID ProcessID,
	reason string,
) (*PreparedWaitingSubtreeCancellation, error) {
	if engine == nil || !rootID.Valid() || !targetID.Valid() {
		return nil, ErrInvalidProcessRelation
	}
	if err := validateTerminationReason(reason); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidProcessControl, err)
	}
	ctx = contextOrBackground(ctx)
	operation, err := engine.acquireTreeOperation(ctx, rootID)
	if err != nil {
		return nil, err
	}
	quiescence, err := engine.quiesceTree(ctx, rootID)
	if err != nil {
		operation.release()
		return nil, err
	}
	release := true
	defer func() {
		if release {
			quiescence.release()
			operation.release()
		}
	}()
	source, err := engine.captureQuiescedTree(ctx, rootID, quiescence.controllers)
	if err != nil {
		return nil, err
	}
	result, canceled, paused, err := projectWaitingSubtreeCancellation(
		source, targetID, reason, time.Now().Round(0).UTC(),
	)
	if err != nil {
		return nil, err
	}
	if !validWaitingSubtreeCancellationProjection(result, canceled, paused) {
		return nil, ErrInvalidPreparedWaitingSubtreeCancellation
	}
	applyGate := make(chan struct{})
	stateChanges, err := newPreparedProcessStateChanges(
		source, result, canceled, paused, applyGate,
	)
	if err != nil {
		return nil, err
	}
	registrations, err := childWaitRegistrationsFromSnapshot(result)
	if err != nil {
		return nil, err
	}
	prepared := &PreparedWaitingSubtreeCancellation{
		engine: engine, operation: operation, quiescence: quiescence,
		resultingSnapshot: result, canceledProcessIDs: canceled, pausedProcessIDs: paused,
		preparedStateChanges: stateChanges, childWaitRegistrations: registrations,
		applyGate: applyGate,
	}
	if !prepared.valid() {
		return nil, ErrInvalidPreparedWaitingSubtreeCancellation
	}
	release = false
	return prepared, nil
}

func validWaitingSubtreeCancellationProjection(
	result TreeSnapshot,
	canceledProcessIDs []ProcessID,
	pausedProcessIDs []ProcessID,
) bool {
	if !result.Valid() || len(canceledProcessIDs) == 0 {
		return false
	}
	processes := make(map[ProcessID]Snapshot)
	for _, snapshot := range result.ProcessSnapshots() {
		processes[snapshot.ProcessID()] = snapshot
	}
	seen := make(map[ProcessID]struct{}, len(canceledProcessIDs)+len(pausedProcessIDs))
	var previousDepth uint32
	var previousProcessID string
	for index, processID := range canceledProcessIDs {
		snapshot, exists := processes[processID]
		if !exists || snapshot.Status() != StatusCanceled {
			return false
		}
		if _, duplicate := seen[processID]; duplicate {
			return false
		}
		depth := snapshot.Relation().Depth()
		if index > 0 {
			if depth < previousDepth || depth == previousDepth && processID.String() <= previousProcessID {
				return false
			}
		}
		previousDepth = depth
		previousProcessID = processID.String()
		seen[processID] = struct{}{}
	}
	for _, processID := range pausedProcessIDs {
		snapshot, exists := processes[processID]
		if !exists || snapshot.Status() != StatusPaused {
			return false
		}
		if _, duplicate := seen[processID]; duplicate {
			return false
		}
		seen[processID] = struct{}{}
	}
	return true
}
