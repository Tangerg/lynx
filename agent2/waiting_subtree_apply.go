package agent2

import (
	"bytes"
	"context"
	"encoding/json"
	"time"
)

type plannedProcessEvent struct {
	name    string
	payload json.RawMessage
}

type plannedProcessState struct {
	processID ProcessID
	source    Snapshot
	result    processSnapshotWire
	mailbox   signalMailbox
	control   pendingControl
	events    []plannedProcessEvent
	applyGate <-chan struct{}
	applied   chan struct{}
}

func newPlannedProcessStates(
	source TreeSnapshot,
	plan WaitingSubtreeCancellationPlan,
	applyGate <-chan struct{},
) ([]*plannedProcessState, error) {
	sourceByID := make(map[ProcessID]Snapshot)
	for _, snapshot := range source.ProcessSnapshots() {
		sourceByID[snapshot.ProcessID()] = snapshot
	}
	resultByID := make(map[ProcessID]Snapshot)
	for _, snapshot := range plan.resultingSnapshot.ProcessSnapshots() {
		resultByID[snapshot.ProcessID()] = snapshot
	}
	if len(sourceByID) != len(resultByID) {
		return nil, ErrInvalidWaitingSubtreeCancellationPlan
	}
	type plannedProcessChange struct {
		id       ProcessID
		canceled bool
	}
	ordered := make([]plannedProcessChange, 0, len(plan.canceledProcessIDs)+len(plan.pausedProcessIDs))
	for _, id := range plan.canceledProcessIDs {
		ordered = append(ordered, plannedProcessChange{id: id, canceled: true})
	}
	for _, id := range plan.pausedProcessIDs {
		ordered = append(ordered, plannedProcessChange{id: id})
	}
	plannedStates := make([]*plannedProcessState, 0, len(ordered))
	for _, change := range ordered {
		sourceSnapshot := sourceByID[change.id]
		resultSnapshot := resultByID[change.id]
		if !sourceSnapshot.Valid() || !resultSnapshot.Valid() {
			return nil, ErrInvalidWaitingSubtreeCancellationPlan
		}
		sourceWire, err := sourceSnapshot.wire()
		if err != nil {
			return nil, err
		}
		resultWire, err := resultSnapshot.wire()
		if err != nil {
			return nil, err
		}
		mailbox, err := restoreSignalMailbox(resultWire.Mailbox)
		if err != nil {
			return nil, err
		}
		control, err := pendingControlFromWire(resultWire.PendingControl)
		if err != nil {
			return nil, err
		}
		plannedState := &plannedProcessState{
			processID: change.id, source: sourceSnapshot, result: resultWire,
			mailbox: mailbox, control: control, applyGate: applyGate, applied: make(chan struct{}),
		}
		if change.canceled {
			if resultWire.Status != StatusCanceled ||
				resultWire.ProcessEventSequence != sourceWire.ProcessEventSequence+1 {
				return nil, ErrInvalidWaitingSubtreeCancellationPlan
			}
		} else {
			if resultWire.Status != StatusPaused || len(resultWire.Mailbox.Signals) < len(sourceWire.Mailbox.Signals) {
				return nil, ErrInvalidWaitingSubtreeCancellationPlan
			}
			for _, record := range resultWire.Mailbox.Signals[len(sourceWire.Mailbox.Signals):] {
				if _, err := ParseChildrenCompleted(record.Signal); err != nil {
					return nil, ErrInvalidWaitingSubtreeCancellationPlan
				}
				payload, _ := json.Marshal(signalAcceptedEventPayload{
					SignalID: record.Signal.ID().String(), WaitID: commandSignalWaitID(record.Signal),
				})
				plannedState.events = append(plannedState.events, plannedProcessEvent{
					name: EventSignalAccepted, payload: payload,
				})
			}
			plannedState.events = append(plannedState.events, plannedProcessEvent{
				name: EventProcessPaused, payload: emptyEventPayload(),
			})
			if resultWire.ProcessEventSequence != sourceWire.ProcessEventSequence+uint64(len(plannedState.events)) {
				return nil, ErrInvalidWaitingSubtreeCancellationPlan
			}
		}
		plannedStates = append(plannedStates, plannedState)
	}
	return plannedStates, nil
}

func (plannedState *plannedProcessState) validateSource(loop *processLoop) error {
	if plannedState == nil || loop == nil || plannedState.processID != loop.controller.processID ||
		loop.status.Terminal() {
		return ErrWaitingSubtreeCancellationPlanStale
	}
	current, err := loop.capture()
	if err != nil || !bytes.Equal(current.JSON(), plannedState.source.JSON()) {
		return ErrWaitingSubtreeCancellationPlanStale
	}
	return nil
}

func (plannedState *plannedProcessState) apply(ctx context.Context, loop *processLoop) {
	result := plannedState.result
	loop.startedAt = result.StartedAt
	loop.status = result.Status
	loop.committedSteps = result.CommittedSteps
	loop.lastStableState = result.LastStableState.clone()
	loop.mailbox = plannedState.mailbox
	loop.prepared = nil
	loop.currentWaitID = WaitID{}
	if result.CurrentWaitID != nil {
		loop.currentWaitID = *result.CurrentWaitID
	}
	loop.pauseReason = result.PauseReason
	loop.pendingControl = plannedState.control
	loop.finalOutput = Output{}
	if result.Output != nil {
		loop.finalOutput = *result.Output
	}
	loop.finishedAt = time.Time{}
	if result.FinishedAt != nil {
		loop.finishedAt = *result.FinishedAt
	}
	loop.termination = Termination{}
	if result.Termination != nil {
		loop.termination = *result.Termination
	}
	loop.reservedBudget = result.ReservedBudget
	loop.usage = result.Usage
	loop.processEventSequence = result.ProcessEventSequence - uint64(len(plannedState.events))
	if result.Status.Terminal() {
		loop.processEventSequence--
	}
	loop.updateView()
	for _, event := range plannedState.events {
		loop.publishEvent(ctx, event.name, EventPhaseCommitted, 0, EffectID{}, event.payload)
	}
}

func childWaitRegistrationsFromSnapshot(
	snapshot TreeSnapshot,
) ([]*childWaitRegistration, error) {
	wire, err := snapshot.wire()
	if err != nil {
		return nil, err
	}
	processes, _, err := processSnapshotWires(wire.ProcessSnapshots)
	if err != nil {
		return nil, err
	}
	registrations := make([]*childWaitRegistration, 0, len(wire.ChildWaits))
	for _, encoded := range wire.ChildWaits {
		spec, err := encoded.Spec.value()
		if err != nil {
			return nil, err
		}
		parent := processes[encoded.ParentProcessID]
		mailbox, err := restoreSignalMailbox(parent.Mailbox)
		if err != nil {
			return nil, err
		}
		registrations = append(registrations, &childWaitRegistration{
			parent: encoded.ParentProcessID, waitID: encoded.WaitID, spec: spec,
			delivered: mailbox.contains(deriveChildCompletionSignalID(encoded.WaitID)),
		})
	}
	return registrations, nil
}

func (engine *Engine) replaceTreeChildWaits(
	rootID ProcessID,
	registrations []*childWaitRegistration,
) {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	for waitID, registration := range engine.childWaits {
		parent := engine.processes[registration.parent]
		if parent != nil && parent.relation.RootID() == rootID {
			delete(engine.childWaits, waitID)
		}
	}
	for _, registration := range registrations {
		engine.childWaits[registration.waitID] = registration
	}
}

func controllerByID(controllers []*processController, id ProcessID) *processController {
	for _, controller := range controllers {
		if controller.processID == id {
			return controller
		}
	}
	return nil
}
