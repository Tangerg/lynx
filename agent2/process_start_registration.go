package agent2

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

func (engine *Engine) reserveProcessStart(
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
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if engine.closed {
		return ErrEngineClosed
	}
	processID := relation.ProcessID()
	if _, exists := engine.processes[processID]; exists {
		return ErrProcessAlreadyExists
	}
	if _, exists := engine.startReservations[processID]; exists {
		return ErrProcessAlreadyExists
	}
	if relation.IsRoot() {
		if childRequestDigest.Valid() {
			return ErrInvalidProcessRelation
		}
		engine.startReservations[processID] = reservation
		return nil
	}
	if !childRequestDigest.Valid() {
		return ErrInvalidProcessRelation
	}
	parentID, child := relation.ParentID()
	key, keyed := relation.ChildKey()
	if !child || !keyed {
		return ErrInvalidProcessRelation
	}
	identity := childIdentity{parent: parentID, key: key}
	if _, exists := engine.children[identity]; exists {
		return ErrInvalidChildStart
	}
	if _, exists := engine.childStartReservations[identity]; exists {
		return ErrInvalidChildStart
	}
	parent := engine.processes[parentID]
	if parent == nil {
		return ErrInvalidProcessRelation
	}
	if treeLimits != parent.treeLimits || relation.depth > treeLimits.MaxDepth {
		return ErrResourceLimitExceeded
	}
	var childCount, activeChildCount, treeProcessCount uint32
	for _, existing := range engine.processes {
		if existing.relation.rootID == relation.rootID {
			treeProcessCount++
		}
		if existing.relation.parentID == parentID {
			childCount++
			if !existing.status().Terminal() {
				activeChildCount++
			}
		}
	}
	for _, pending := range engine.startReservations {
		if pending.relation.rootID == relation.rootID {
			treeProcessCount++
		}
		if pending.relation.parentID == parentID {
			childCount++
			activeChildCount++
		}
	}
	if childCount >= treeLimits.MaxChildren ||
		activeChildCount >= treeLimits.MaxActiveChildren ||
		treeProcessCount >= treeLimits.MaxTreeProcesses {
		return ErrResourceLimitExceeded
	}
	engine.startReservations[processID] = reservation
	engine.childStartReservations[identity] = processID
	return nil
}

func (engine *Engine) discardProcessStartReservation(processID ProcessID) {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	reservation, exists := engine.startReservations[processID]
	if !exists {
		return
	}
	delete(engine.startReservations, processID)
	if parentID, child := reservation.relation.ParentID(); child {
		key, _ := reservation.relation.ChildKey()
		identity := childIdentity{parent: parentID, key: key}
		if engine.childStartReservations[identity] == processID {
			delete(engine.childStartReservations, identity)
		}
	}
}

func (engine *Engine) publishReservedProcess(controller *processController) {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	reservation, exists := engine.startReservations[controller.processID]
	if !exists || reservation.relation != controller.relation ||
		reservation.deploymentRef != controller.deploymentRef ||
		reservation.treeLimits != controller.treeLimits || engine.closed ||
		engine.processes[controller.processID] != nil {
		panic("agent: invalid Process start reservation")
	}
	var identity childIdentity
	parentID, isChild := controller.relation.ParentID()
	if isChild {
		key, _ := controller.relation.ChildKey()
		identity = childIdentity{parent: parentID, key: key}
		if engine.childStartReservations[identity] != controller.processID ||
			engine.children[identity].Valid() {
			panic("agent: invalid child Process start reservation")
		}
	}
	delete(engine.startReservations, controller.processID)
	controller.childRequestDigest = reservation.childRequestDigest
	engine.processes[controller.processID] = controller
	if isChild {
		delete(engine.childStartReservations, identity)
		engine.children[identity] = controller.processID
	}
}

func (engine *Engine) acknowledgeStartedProcessOutcome(
	ctx context.Context,
	admission ProcessAdmission,
) error {
	if err := acknowledgeProcessStartOutcome(
		engine.startOutcomeAcknowledger, ctx, startedProcessOutcome(admission),
	); err != nil {
		return fmt.Errorf("agent: acknowledge started Process: %w", err)
	}
	return nil
}

func (engine *Engine) acknowledgeAbortedProcessOutcome(
	ctx context.Context,
	admission ProcessAdmission,
	failure Failure,
) error {
	if err := acknowledgeProcessStartOutcome(
		engine.startOutcomeAcknowledger, ctx, abortedProcessOutcome(admission, failure),
	); err != nil {
		return fmt.Errorf("agent: acknowledge aborted Process: %w", err)
	}
	return nil
}
