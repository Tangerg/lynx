package agent

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
	ErrInvalidPreparedWaitingSubtreeCancellation = errors.New("agent: invalid prepared waiting subtree cancellation")

	ErrPreparedWaitingSubtreeCancellationResolved = errors.New("agent: prepared waiting subtree cancellation is resolved")

	ErrWaitingSubtreeCancellationUnavailable = errors.New("agent: waiting subtree cancellation is unavailable")
)

// PreparedWaitingSubtreeCancellation owns one frozen, Strategy-safe source
// tree and its exact prospective cancellation result. Callers must retain the
// returned pointer rather than copy the value. The underlying authority is
// shared and must be resolved exactly once with Apply or Discard; an accidental
// value copy does not duplicate that authority. Until resolution, the source
// tree remains frozen. Other root trees in the Engine remain independent.
type PreparedWaitingSubtreeCancellation struct {
	engine                 *Engine
	source                 *quiescedTree
	resultingSnapshot      TreeSnapshot
	canceledProcessIDs     []ProcessID
	pausedProcessIDs       []ProcessID
	preparedStateChanges   []*preparedProcessStateChange
	childWaitRegistrations []*childWaitRegistration
	applyGate              chan struct{}

	resolution *waitingSubtreeCancellationResolution
}

// waitingSubtreeCancellationResolution is the identity of the one-shot
// authority. Keeping it behind a pointer makes resolution linearizable even
// when a caller accidentally copies the exported prepared value.
type waitingSubtreeCancellationResolution struct {
	mu       sync.Mutex
	resolved bool
}

// ResultingSnapshot returns the exact complete tree state that Apply will
// install. The snapshot remains readable after Apply or Discard.
func (p *PreparedWaitingSubtreeCancellation) ResultingSnapshot() TreeSnapshot {
	if p == nil {
		return TreeSnapshot{}
	}
	return p.resultingSnapshot
}

// CanceledProcessIDs returns Processes projected as Canceled, ordered from
// parent to child and then by ProcessID within one depth.
func (p *PreparedWaitingSubtreeCancellation) CanceledProcessIDs() []ProcessID {
	if p == nil {
		return nil
	}
	return slices.Clone(p.canceledProcessIDs)
}

// PausedProcessIDs returns parents projected as Paused before they can consume
// a child-completion Signal. A caller that later continues uses Process.Resume.
func (p *PreparedWaitingSubtreeCancellation) PausedProcessIDs() []ProcessID {
	if p == nil {
		return nil
	}
	return slices.Clone(p.pausedProcessIDs)
}

// Apply commits the exact prepared Framework state and releases the frozen
// source tree. Prepare completed every fallible or cancelable operation, so
// Apply deliberately has no context: once the caller's durable decision exists,
// request cancellation cannot revoke this in-memory commit boundary.
func (p *PreparedWaitingSubtreeCancellation) Apply() error {
	if p == nil || p.resolution == nil {
		return ErrInvalidPreparedWaitingSubtreeCancellation
	}
	p.resolution.mu.Lock()
	defer p.resolution.mu.Unlock()
	if p.resolution.resolved {
		return ErrPreparedWaitingSubtreeCancellationResolved
	}
	if !p.valid() {
		p.resolveLocked()
		return ErrInvalidPreparedWaitingSubtreeCancellation
	}
	p.engine.replaceTreeChildWaits(
		p.resultingSnapshot.RootID(), p.childWaitRegistrations,
	)
	close(p.applyGate)
	for _, change := range p.preparedStateChanges {
		<-change.applied
	}
	p.source.quiescence.release()
	for _, processID := range p.canceledProcessIDs {
		controller := controllerByID(p.source.quiescence.controllers, processID)
		if controller != nil {
			_ = waitTreeSettled(context.Background(), controller)
		}
	}
	p.resolveLocked()
	return nil
}

// Discard releases the frozen source tree without applying the prospective
// cancellation. It returns an error when Apply or Discard already resolved it.
func (p *PreparedWaitingSubtreeCancellation) Discard() error {
	if p == nil || p.resolution == nil {
		return ErrInvalidPreparedWaitingSubtreeCancellation
	}
	p.resolution.mu.Lock()
	defer p.resolution.mu.Unlock()
	if p.resolution.resolved {
		return ErrPreparedWaitingSubtreeCancellationResolved
	}
	if !p.valid() {
		p.resolveLocked()
		return ErrInvalidPreparedWaitingSubtreeCancellation
	}
	p.resolveLocked()
	return nil
}

func (p *PreparedWaitingSubtreeCancellation) valid() bool {
	return p.engine != nil &&
		p.source.valid(p.engine, p.resultingSnapshot.RootID()) &&
		p.resultingSnapshot.Valid() &&
		len(p.canceledProcessIDs) > 0 &&
		len(p.preparedStateChanges) ==
			len(p.canceledProcessIDs)+len(p.pausedProcessIDs) &&
		p.applyGate != nil && p.resolution != nil &&
		validWaitingSubtreeCancellationProjection(
			p.resultingSnapshot,
			p.canceledProcessIDs,
			p.pausedProcessIDs,
		)
}

func (p *PreparedWaitingSubtreeCancellation) resolveLocked() {
	p.source.release()
	p.engine = nil
	p.source = nil
	p.preparedStateChanges = nil
	p.childWaitRegistrations = nil
	p.applyGate = nil
	p.resolution.resolved = true
}

// PrepareWaitingSubtreeCancellation freezes one complete source tree and
// computes its exact cancellation result. targetID must identify a non-root
// Waiting Process in the tree rooted at rootID. The returned capability must be
// resolved exactly once with Apply or Discard.
func (e *Engine) PrepareWaitingSubtreeCancellation(
	ctx context.Context,
	rootID ProcessID,
	targetID ProcessID,
	reason string,
) (*PreparedWaitingSubtreeCancellation, error) {
	if e == nil || !rootID.Valid() || !targetID.Valid() {
		return nil, ErrInvalidProcessRelation
	}
	if err := validateTerminationReason(reason); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidProcessControl, err)
	}
	ctx = contextOrBackground(ctx)
	sourceTree, err := e.quiesceOwnedTree(ctx, rootID)
	if err != nil {
		return nil, err
	}
	var prepared *PreparedWaitingSubtreeCancellation
	defer func() {
		if prepared == nil {
			sourceTree.release()
		}
	}()
	source, err := e.captureQuiescedTree(ctx, rootID, sourceTree.quiescence.controllers)
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
	prepared = &PreparedWaitingSubtreeCancellation{
		engine: e, source: sourceTree,
		resultingSnapshot: result, canceledProcessIDs: canceled, pausedProcessIDs: paused,
		preparedStateChanges: stateChanges, childWaitRegistrations: registrations,
		applyGate: applyGate, resolution: &waitingSubtreeCancellationResolution{},
	}
	if !prepared.valid() {
		return nil, ErrInvalidPreparedWaitingSubtreeCancellation
	}
	if err := stagePreparedProcessStateChanges(ctx, sourceTree.quiescence, stateChanges); err != nil {
		return nil, err
	}
	return prepared, nil
}

func stagePreparedProcessStateChanges(
	ctx context.Context,
	quiescence *treeQuiescence,
	changes []*preparedProcessStateChange,
) error {
	for _, change := range changes {
		controller := controllerByID(quiescence.controllers, change.processID)
		if controller == nil {
			return ErrEngineQuiescenceUnavailable
		}
		response, err := (&Process{controller: controller}).request(ctx, processCommand{
			kind: commandStagePreparedProcessState, preparedStateChange: change,
		})
		if err != nil {
			return err
		}
		if !response.accepted {
			return ErrEngineQuiescenceUnavailable
		}
	}
	return nil
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
