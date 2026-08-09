package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"time"
)

type preparedProcessEvent struct {
	name    string
	payload json.RawMessage
}

type preparedProcessStateChange struct {
	processID ProcessID
	source    Snapshot
	result    processSnapshotWire
	mailbox   signalMailbox
	control   pendingControl
	events    []preparedProcessEvent
	applyGate <-chan struct{}
	applied   chan struct{}
}

func newPreparedProcessStateChanges(
	source TreeSnapshot,
	result TreeSnapshot,
	canceledProcessIDs []ProcessID,
	pausedProcessIDs []ProcessID,
	applyGate <-chan struct{},
) ([]*preparedProcessStateChange, error) {
	sourceByID := snapshotsByProcessID(source)
	resultByID := snapshotsByProcessID(result)
	if len(sourceByID) != len(resultByID) {
		return nil, ErrInvalidPreparedWaitingSubtreeCancellation
	}
	ordered := orderedPreparedProcessChanges(canceledProcessIDs, pausedProcessIDs)
	preparedChanges := make([]*preparedProcessStateChange, 0, len(ordered))
	for _, change := range ordered {
		sourceSnapshot := sourceByID[change.id]
		resultSnapshot := resultByID[change.id]
		if !sourceSnapshot.Valid() || !resultSnapshot.Valid() {
			return nil, ErrInvalidPreparedWaitingSubtreeCancellation
		}
		preparedChange, err := newPreparedProcessStateChange(
			sourceSnapshot, resultSnapshot, change.canceled, applyGate,
		)
		if err != nil {
			return nil, err
		}
		preparedChanges = append(preparedChanges, preparedChange)
	}
	return preparedChanges, nil
}

type preparedProcessChange struct {
	id       ProcessID
	canceled bool
}

func snapshotsByProcessID(snapshot TreeSnapshot) map[ProcessID]Snapshot {
	byID := make(map[ProcessID]Snapshot)
	for _, processSnapshot := range snapshot.ProcessSnapshots() {
		byID[processSnapshot.ProcessID()] = processSnapshot
	}
	return byID
}

func orderedPreparedProcessChanges(canceled, paused []ProcessID) []preparedProcessChange {
	ordered := make([]preparedProcessChange, 0, len(canceled)+len(paused))
	for _, id := range canceled {
		ordered = append(ordered, preparedProcessChange{id: id, canceled: true})
	}
	for _, id := range paused {
		ordered = append(ordered, preparedProcessChange{id: id})
	}
	return ordered
}

func newPreparedProcessStateChange(
	source Snapshot,
	result Snapshot,
	canceled bool,
	applyGate <-chan struct{},
) (*preparedProcessStateChange, error) {
	sourceWire, err := source.wire()
	if err != nil {
		return nil, err
	}
	resultWire, err := result.wire()
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
	prepared := &preparedProcessStateChange{
		processID: result.ProcessID(), source: source, result: resultWire,
		mailbox: mailbox, control: control, applyGate: applyGate, applied: make(chan struct{}),
	}
	if canceled {
		if resultWire.Status != StatusCanceled ||
			resultWire.ProcessEventSequence != sourceWire.ProcessEventSequence+1 {
			return nil, ErrInvalidPreparedWaitingSubtreeCancellation
		}
		return prepared, nil
	}
	if err := prepared.preparePauseEvents(sourceWire, resultWire); err != nil {
		return nil, err
	}
	return prepared, nil
}

func (prepared *preparedProcessStateChange) preparePauseEvents(
	source processSnapshotWire,
	result processSnapshotWire,
) error {
	if result.Status != StatusPaused || len(result.Mailbox.Signals) < len(source.Mailbox.Signals) {
		return ErrInvalidPreparedWaitingSubtreeCancellation
	}
	for _, record := range result.Mailbox.Signals[len(source.Mailbox.Signals):] {
		if _, err := ParseChildrenCompleted(record.Signal); err != nil {
			return ErrInvalidPreparedWaitingSubtreeCancellation
		}
		payload, _ := json.Marshal(signalAcceptedEventPayload{
			SignalID: record.Signal.ID().String(), WaitID: commandSignalWaitID(record.Signal),
		})
		prepared.events = append(prepared.events, preparedProcessEvent{
			name: EventSignalAccepted, payload: payload,
		})
	}
	prepared.events = append(prepared.events, preparedProcessEvent{
		name: EventProcessPaused, payload: emptyEventPayload(),
	})
	if result.ProcessEventSequence != source.ProcessEventSequence+uint64(len(prepared.events)) {
		return ErrInvalidPreparedWaitingSubtreeCancellation
	}
	return nil
}

func (preparedChange *preparedProcessStateChange) validateSource(loop *processLoop) error {
	if preparedChange == nil || loop == nil || preparedChange.processID != loop.controller.processID ||
		loop.status.Terminal() {
		return ErrInvalidPreparedWaitingSubtreeCancellation
	}
	current, err := loop.capture()
	if err != nil || !bytes.Equal(current.JSON(), preparedChange.source.JSON()) {
		return ErrInvalidPreparedWaitingSubtreeCancellation
	}
	return nil
}

func (preparedChange *preparedProcessStateChange) apply(ctx context.Context, loop *processLoop) {
	result := preparedChange.result
	loop.startedAt = result.StartedAt
	loop.status = result.Status
	loop.committedSteps = result.CommittedSteps
	loop.lastStableState = result.LastStableState.clone()
	loop.mailbox = preparedChange.mailbox
	loop.prepared = nil
	loop.currentWaitID = WaitID{}
	if result.CurrentWaitID != nil {
		loop.currentWaitID = *result.CurrentWaitID
	}
	loop.pauseReason = result.PauseReason
	loop.pendingControl = preparedChange.control
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
	loop.processEventSequence = result.ProcessEventSequence - uint64(len(preparedChange.events))
	if result.Status.Terminal() {
		loop.processEventSequence--
	}
	loop.updateView()
	for _, event := range preparedChange.events {
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
