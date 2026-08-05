package agent2

type childWaitRegistration struct {
	parent    ProcessID
	waitID    WaitID
	spec      ChildWaitSpec
	delivered bool
}

type childCompletionDelivery struct {
	parent ProcessID
	signal Signal
}

func (engine *Engine) registerChildWait(
	parent ProcessID,
	waitID WaitID,
	spec ChildWaitSpec,
) (*Signal, error) {
	if !parent.Valid() || !waitID.Valid() || !spec.Valid() {
		return nil, ErrInvalidChildWait
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if _, duplicate := engine.childWaits[waitID]; duplicate {
		return nil, ErrInvalidChildWait
	}
	if _, exists := engine.processes[parent]; !exists {
		return nil, ErrInvalidProcessRelation
	}
	for _, childID := range spec.Children {
		controller, exists := engine.processes[childID]
		if !exists {
			return nil, ErrInvalidChildWait
		}
		actualParent, child := controller.relation.ParentID()
		if !child || actualParent != parent {
			return nil, ErrInvalidChildWait
		}
	}
	registration := &childWaitRegistration{
		parent: parent, waitID: waitID, spec: cloneChildWaitSpec(spec),
	}
	engine.childWaits[waitID] = registration
	outcomes, ready := engine.childWaitOutcomesLocked(registration)
	if !ready {
		return nil, nil
	}
	signal, err := encodeChildrenCompleted(waitID, spec.Key, outcomes)
	if err != nil {
		delete(engine.childWaits, waitID)
		return nil, err
	}
	registration.delivered = true
	return &signal, nil
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
	for _, candidate := range engine.processes {
		parentID, child := candidate.relation.ParentID()
		if child && parentID == controller.id && !candidate.status().Terminal() {
			activeChildren = append(activeChildren, candidate)
		}
	}
	for _, registration := range engine.childWaits {
		if registration.delivered || !childWaitContains(registration.spec, controller.id) {
			continue
		}
		outcomes, ready := engine.childWaitOutcomesLocked(registration)
		if !ready {
			continue
		}
		signal, err := encodeChildrenCompleted(registration.waitID, registration.spec.Key, outcomes)
		if err != nil {
			continue
		}
		registration.delivered = true
		deliveries = append(deliveries, childCompletionDelivery{
			parent: registration.parent, signal: signal,
		})
	}
	engine.mu.Unlock()
	for _, delivery := range deliveries {
		engine.deliverChildCompletion(delivery)
	}
	parentTermination := controller.terminalResult().Termination()
	for _, child := range activeChildren {
		engine.deliverParentTermination(child, parentTermination)
	}
}

func (*Engine) deliverParentTermination(child *processController, parent Termination) {
	select {
	case child.commands <- processCommand{
		kind: commandParentTerminated, parentTermination: parent,
	}:
	case <-child.done:
	}
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

func (engine *Engine) deliverChildCompletion(delivery childCompletionDelivery) {
	engine.mu.RLock()
	controller := engine.processes[delivery.parent]
	engine.mu.RUnlock()
	if controller == nil {
		return
	}
	select {
	case controller.commands <- processCommand{
		kind: commandChildrenCompleted, internalSignal: delivery.signal,
	}:
	case <-controller.done:
	}
}

func childWaitContains(spec ChildWaitSpec, childID ProcessID) bool {
	for _, candidate := range spec.Children {
		if candidate == childID {
			return true
		}
	}
	return false
}
