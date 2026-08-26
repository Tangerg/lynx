package agent

import (
	"context"
	"fmt"
)

type processStartReservation struct {
	relation           ProcessRelation
	deploymentRef      DeploymentRef
	treeLimits         TreeLimits
	childRequestDigest Digest
}

func (e *Engine) reserveProcessStart(
	relation ProcessRelation,
	deploymentRef DeploymentRef,
	treeLimits TreeLimits,
	childRequestDigest Digest,
) error {
	if !relation.Valid() || !deploymentRef.Valid() || !treeLimits.Valid() {
		return ErrInvalidProcessRelation
	}
	reservation := processStartReservation{
		relation:           relation,
		deploymentRef:      deploymentRef,
		treeLimits:         treeLimits,
		childRequestDigest: childRequestDigest,
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return ErrEngineClosed
	}
	processID := relation.ProcessID()
	if _, exists := e.processes[processID]; exists {
		return ErrProcessAlreadyExists
	}
	if _, exists := e.startReservations[processID]; exists {
		return ErrProcessAlreadyExists
	}
	if relation.IsRoot() {
		return e.reserveRootStart(reservation)
	}
	return e.reserveChildStart(reservation)
}

// reserveRootStart requires e.mu to be held.
func (e *Engine) reserveRootStart(reservation processStartReservation) error {
	if reservation.childRequestDigest.Valid() {
		return ErrInvalidProcessRelation
	}
	e.startReservations[reservation.relation.ProcessID()] = reservation
	return nil
}

// reserveChildStart requires e.mu to be held.
func (e *Engine) reserveChildStart(reservation processStartReservation) error {
	relation := reservation.relation
	treeLimits := reservation.treeLimits
	if !reservation.childRequestDigest.Valid() {
		return ErrInvalidProcessRelation
	}
	parentID, child := relation.ParentID()
	key, keyed := relation.ChildKey()
	if !child || !keyed {
		return ErrInvalidProcessRelation
	}
	identity := childIdentity{parent: parentID, key: key}
	if _, exists := e.children[identity]; exists {
		return ErrInvalidChildStart
	}
	if _, exists := e.childStartReservations[identity]; exists {
		return ErrInvalidChildStart
	}
	parent := e.processes[parentID]
	if parent == nil {
		return ErrInvalidProcessRelation
	}
	if treeLimits != parent.treeLimits || relation.depth > treeLimits.MaxDepth {
		return ErrResourceLimitExceeded
	}
	childCount, activeChildCount, treeProcessCount := e.reservedTreeCounts(relation.rootID, parentID)
	if childCount >= treeLimits.MaxChildren ||
		activeChildCount >= treeLimits.MaxActiveChildren ||
		treeProcessCount >= treeLimits.MaxTreeProcesses {
		return ErrResourceLimitExceeded
	}
	processID := relation.ProcessID()
	e.startReservations[processID] = reservation
	e.childStartReservations[identity] = processID
	return nil
}

// reservedTreeCounts requires e.mu to be held.
func (e *Engine) reservedTreeCounts(rootID, parentID ProcessID) (
	childCount uint32,
	activeChildCount uint32,
	treeProcessCount uint32,
) {
	for _, existing := range e.processes {
		if existing.relation.rootID == rootID {
			treeProcessCount++
		}
		if existing.relation.parentID == parentID {
			childCount++
			if !existing.status().Terminal() {
				activeChildCount++
			}
		}
	}
	for _, pending := range e.startReservations {
		if pending.relation.rootID == rootID {
			treeProcessCount++
		}
		if pending.relation.parentID == parentID {
			childCount++
			activeChildCount++
		}
	}
	return childCount, activeChildCount, treeProcessCount
}

func (e *Engine) discardProcessStartReservation(processID ProcessID) {
	e.mu.Lock()
	defer e.mu.Unlock()
	reservation, exists := e.startReservations[processID]
	if !exists {
		return
	}
	delete(e.startReservations, processID)
	if parentID, child := reservation.relation.ParentID(); child {
		key, _ := reservation.relation.ChildKey()
		identity := childIdentity{parent: parentID, key: key}
		if e.childStartReservations[identity] == processID {
			delete(e.childStartReservations, identity)
		}
	}
}

func (e *Engine) publishReservedProcess(controller *processController) {
	e.mu.Lock()
	defer e.mu.Unlock()
	reservation, exists := e.startReservations[controller.processID]
	if !exists || reservation.relation != controller.relation ||
		reservation.deploymentRef != controller.deploymentRef ||
		reservation.treeLimits != controller.treeLimits || e.closed ||
		e.processes[controller.processID] != nil {
		panic("agent: invalid Process start reservation")
	}
	var identity childIdentity
	parentID, isChild := controller.relation.ParentID()
	if isChild {
		key, _ := controller.relation.ChildKey()
		identity = childIdentity{parent: parentID, key: key}
		if e.childStartReservations[identity] != controller.processID ||
			e.children[identity].Valid() {
			panic("agent: invalid child Process start reservation")
		}
	}
	delete(e.startReservations, controller.processID)
	controller.childRequestDigest = reservation.childRequestDigest
	e.processes[controller.processID] = controller
	if isChild {
		delete(e.childStartReservations, identity)
		e.children[identity] = controller.processID
	}
}

func (e *Engine) acknowledgeStartedProcessOutcome(
	ctx context.Context,
	admission ProcessAdmission,
) error {
	if err := acknowledgeProcessStartOutcome(
		ctx, e.startOutcomeAcknowledger, startedProcessOutcome(admission),
	); err != nil {
		return fmt.Errorf("agent: acknowledge started Process: %w", err)
	}
	return nil
}

func (e *Engine) acknowledgeAbortedProcessOutcome(
	ctx context.Context,
	admission ProcessAdmission,
	failure Failure,
) error {
	if err := acknowledgeProcessStartOutcome(
		ctx, e.startOutcomeAcknowledger, abortedProcessOutcome(admission, failure),
	); err != nil {
		return fmt.Errorf("agent: acknowledge aborted Process: %w", err)
	}
	return nil
}
