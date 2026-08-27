package agent

import (
	"cmp"
	"context"
	"slices"
)

type childWaitRegistration struct {
	parent    ProcessID
	waitID    WaitID
	spec      ChildWaitSpec
	delivered bool
}

type childCompletionDelivery struct {
	parent ProcessID
	waitID WaitID
	signal Signal
}

func (e *Engine) registerChildWait(
	parent ProcessID,
	waitID WaitID,
	spec ChildWaitSpec,
) (Signal, bool, error) {
	if !parent.Valid() || !waitID.Valid() || !spec.Valid() {
		return Signal{}, false, ErrInvalidChildWait
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, duplicate := e.childWaits[waitID]; duplicate {
		return Signal{}, false, ErrInvalidChildWait
	}
	if _, exists := e.processes[parent]; !exists {
		return Signal{}, false, ErrInvalidProcessRelation
	}
	for _, childID := range spec.Children {
		controller, exists := e.processes[childID]
		if !exists {
			return Signal{}, false, ErrInvalidChildWait
		}
		actualParent, child := controller.relation.ParentID()
		if !child || actualParent != parent {
			return Signal{}, false, ErrInvalidChildWait
		}
	}
	registration := &childWaitRegistration{
		parent: parent, waitID: waitID, spec: cloneChildWaitSpec(spec),
	}
	e.childWaits[waitID] = registration
	outcomes, satisfied := e.childWaitOutcomesLocked(registration)
	if !satisfied {
		return Signal{}, false, nil
	}
	signal, err := encodeChildrenCompleted(waitID, spec.Key, outcomes)
	if err != nil {
		delete(e.childWaits, waitID)
		return Signal{}, false, err
	}
	registration.delivered = true
	return signal, true, nil
}

func (e *Engine) unregisterChildWait(waitID WaitID) {
	e.mu.Lock()
	delete(e.childWaits, waitID)
	e.mu.Unlock()
}

func (e *Engine) processFinished(controller *processController) {
	if e == nil || controller == nil {
		return
	}
	var deliveries []childCompletionDelivery
	var activeChildren []*processController
	e.mu.Lock()
	for waitID, registration := range e.childWaits {
		if registration.parent == controller.processID {
			delete(e.childWaits, waitID)
		}
	}
	for _, candidate := range e.processes {
		parentID, child := candidate.relation.ParentID()
		if child && parentID == controller.processID && !candidate.status().Terminal() {
			activeChildren = append(activeChildren, candidate)
		}
	}
	for _, registration := range e.childWaits {
		if registration.delivered || !slices.Contains(registration.spec.Children, controller.processID) {
			continue
		}
		outcomes, satisfied := e.childWaitOutcomesLocked(registration)
		if !satisfied {
			continue
		}
		signal, err := encodeChildrenCompleted(registration.waitID, registration.spec.Key, outcomes)
		if err != nil {
			continue
		}
		deliveries = append(deliveries, childCompletionDelivery{
			parent: registration.parent, waitID: registration.waitID, signal: signal,
		})
	}
	e.mu.Unlock()
	sortChildCompletionDeliveries(deliveries)
	for _, delivery := range deliveries {
		if e.deliverChildCompletion(delivery) {
			e.mu.Lock()
			if registration := e.childWaits[delivery.waitID]; registration != nil {
				registration.delivered = true
			}
			e.mu.Unlock()
		}
	}
	parentTermination := controller.terminalResult().Termination()
	for _, child := range activeChildren {
		e.deliverParentTermination(child, parentTermination)
	}
}

func (*Engine) deliverParentTermination(child *processController, parent Termination) {
	_, _ = (&Process{controller: child}).request(context.Background(), processCommand{
		kind: commandParentTerminated, parentTermination: parent,
	})
}

func (e *Engine) childWaitOutcomesLocked(
	registration *childWaitRegistration,
) ([]ChildOutcome, bool) {
	outcomes := make([]ChildOutcome, 0, len(registration.spec.Children))
	for _, childID := range registration.spec.Children {
		controller := e.processes[childID]
		if controller == nil {
			return nil, false
		}
		select {
		case <-controller.done:
			key, _ := controller.relation.ChildKey()
			outcomes = append(outcomes, ChildOutcome{key: key, result: controller.terminalResult()})
		default:
		}
	}
	required, err := registration.spec.Condition.required(len(registration.spec.Children))
	return outcomes, err == nil && uint32(len(outcomes)) >= required
}

func (e *Engine) deliverChildCompletion(delivery childCompletionDelivery) bool {
	e.mu.RLock()
	controller := e.processes[delivery.parent]
	e.mu.RUnlock()
	if controller == nil {
		return false
	}
	response, err := (&Process{controller: controller}).request(context.Background(), processCommand{
		kind: commandChildrenCompleted, internalSignal: delivery.signal,
	})
	return err == nil && response.accepted
}

func sortChildCompletionDeliveries(deliveries []childCompletionDelivery) {
	slices.SortFunc(deliveries, func(left, right childCompletionDelivery) int {
		return cmp.Compare(left.waitID.String(), right.waitID.String())
	})
}
