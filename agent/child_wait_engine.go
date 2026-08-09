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

func (engine *Engine) registerChildWait(
	parent ProcessID,
	waitID WaitID,
	spec ChildWaitSpec,
) (Signal, bool, error) {
	if !parent.Valid() || !waitID.Valid() || !spec.Valid() {
		return Signal{}, false, ErrInvalidChildWait
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if _, duplicate := engine.childWaits[waitID]; duplicate {
		return Signal{}, false, ErrInvalidChildWait
	}
	if _, exists := engine.processes[parent]; !exists {
		return Signal{}, false, ErrInvalidProcessRelation
	}
	for _, childID := range spec.Children {
		controller, exists := engine.processes[childID]
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
	engine.childWaits[waitID] = registration
	outcomes, satisfied := engine.childWaitOutcomesLocked(registration)
	if !satisfied {
		return Signal{}, false, nil
	}
	signal, err := encodeChildrenCompleted(waitID, spec.Key, outcomes)
	if err != nil {
		delete(engine.childWaits, waitID)
		return Signal{}, false, err
	}
	registration.delivered = true
	return signal, true, nil
}

func (engine *Engine) unregisterChildWait(waitID WaitID) {
	engine.mu.Lock()
	delete(engine.childWaits, waitID)
	engine.mu.Unlock()
}

func (engine *Engine) processFinished(controller *processController) {
	if engine == nil || controller == nil {
		return
	}
	var deliveries []childCompletionDelivery
	var activeChildren []*processController
	engine.mu.Lock()
	for waitID, registration := range engine.childWaits {
		if registration.parent == controller.processID {
			delete(engine.childWaits, waitID)
		}
	}
	for _, candidate := range engine.processes {
		parentID, child := candidate.relation.ParentID()
		if child && parentID == controller.processID && !candidate.status().Terminal() {
			activeChildren = append(activeChildren, candidate)
		}
	}
	for _, registration := range engine.childWaits {
		if registration.delivered || !childWaitContains(registration.spec, controller.processID) {
			continue
		}
		outcomes, satisfied := engine.childWaitOutcomesLocked(registration)
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
	engine.mu.Unlock()
	sortChildCompletionDeliveries(deliveries)
	for _, delivery := range deliveries {
		if engine.deliverChildCompletion(delivery) {
			engine.mu.Lock()
			if registration := engine.childWaits[delivery.waitID]; registration != nil {
				registration.delivered = true
			}
			engine.mu.Unlock()
		}
	}
	parentTermination := controller.terminalResult().Termination()
	for _, child := range activeChildren {
		engine.deliverParentTermination(child, parentTermination)
	}
}

func (*Engine) deliverParentTermination(child *processController, parent Termination) {
	_, _ = (&Process{controller: child}).request(context.Background(), processCommand{
		kind: commandParentTerminated, parentTermination: parent,
	})
}

func (engine *Engine) childWaitOutcomesLocked(
	registration *childWaitRegistration,
) ([]ChildOutcome, bool) {
	outcomes := make([]ChildOutcome, 0, len(registration.spec.Children))
	for _, childID := range registration.spec.Children {
		controller := engine.processes[childID]
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

func (engine *Engine) deliverChildCompletion(delivery childCompletionDelivery) bool {
	engine.mu.RLock()
	controller := engine.processes[delivery.parent]
	engine.mu.RUnlock()
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

func childWaitContains(spec ChildWaitSpec, childID ProcessID) bool {
	for _, candidate := range spec.Children {
		if candidate == childID {
			return true
		}
	}
	return false
}
