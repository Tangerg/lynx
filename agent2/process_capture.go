package agent2

import "fmt"

func prepareRestoredProcess(
	engine *Engine,
	deployment Deployment,
	snapshot Snapshot,
) (*processController, *processLoop, processSnapshotWire, error) {
	wire, err := snapshot.wire()
	if err != nil {
		return nil, nil, processSnapshotWire{}, err
	}
	if wire.DeploymentRef != deployment.DeploymentRef() {
		return nil, nil, processSnapshotWire{}, fmt.Errorf(
			"%w: exact Deployment does not match", ErrInvalidSnapshot,
		)
	}
	if wire.Output != nil {
		if err := deployment.Descriptor().ValidateOutput(*wire.Output); err != nil {
			return nil, nil, processSnapshotWire{}, fmt.Errorf(
				"%w: output schema: %w", ErrInvalidSnapshot, err,
			)
		}
	}
	execution, err := restoreExecution(deployment.Definition(), wire.LastStableState)
	if err != nil {
		return nil, nil, processSnapshotWire{}, fmt.Errorf(
			"%w: restore Execution: %w", ErrInvalidSnapshot, err,
		)
	}
	mailbox, err := restoreSignalMailbox(wire.Mailbox)
	if err != nil {
		return nil, nil, processSnapshotWire{}, fmt.Errorf("%w: mailbox: %w", ErrInvalidSnapshot, err)
	}
	relation, err := processRelationFromWire(wire.ProcessID, wire.Relation)
	if err != nil {
		return nil, nil, processSnapshotWire{}, fmt.Errorf("%w: relation: %w", ErrInvalidSnapshot, err)
	}
	controller := newProcessController(
		relation, wire.DeploymentRef, wire.Budget, wire.Capabilities, wire.TreeLimits,
		wire.StartedAt, wire.Status,
	)
	loop, err := restoreProcessLoop(engine, controller, deployment, execution, mailbox, wire)
	if err != nil {
		return nil, nil, processSnapshotWire{}, err
	}
	return controller, loop, wire, nil
}

func restoreProcessLoop(
	engine *Engine,
	controller *processController,
	deployment Deployment,
	execution Execution,
	mailbox signalMailbox,
	wire processSnapshotWire,
) (*processLoop, error) {
	loop := &processLoop{
		engine: engine, controller: controller, deployment: deployment, execution: execution,
		startedAt: wire.StartedAt, status: wire.Status, committedSteps: wire.CommittedSteps,
		eventSequence: wire.EventSequence, lastStableState: wire.LastStableState, mailbox: mailbox, restored: true,
		pauseReason: wire.PauseReason, limits: wire.Limits, treeLimits: wire.TreeLimits,
		budget: wire.Budget, reservedBudget: wire.ReservedBudget,
		capabilities: wire.Capabilities, usage: wire.Usage,
	}
	if wire.ChildRequestDigest != nil {
		controller.childRequestDigest = *wire.ChildRequestDigest
	}
	if wire.FinishedAt != nil {
		loop.finishedAt = *wire.FinishedAt
	}
	if wire.CurrentWaitID != nil {
		loop.currentWaitID = *wire.CurrentWaitID
	}
	if wire.Output != nil {
		loop.finalOutput = *wire.Output
	}
	if wire.Termination != nil {
		loop.termination = *wire.Termination
	}
	control, err := pendingControlFromWire(wire.PendingControl)
	if err != nil {
		return nil, fmt.Errorf("%w: pending control: %w", ErrInvalidSnapshot, err)
	}
	loop.pendingControl = control
	if wire.Prepared != nil {
		copy := clonePreparedStep(*wire.Prepared)
		loop.prepared = &preparedStep{wire: copy, acknowledged: true, fromSnapshot: true}
	}
	controller.updateView(loop.status, loop.currentWaitID, loop.usage)
	return loop, nil
}

func (loop *processLoop) capture() (Snapshot, error) {
	wire := processSnapshotWire{
		SchemaVersion: processSnapshotSchemaVersion, ProcessID: loop.controller.processID,
		Relation:      loop.controller.relation.wire(),
		DeploymentRef: loop.deployment.DeploymentRef(), StartedAt: loop.startedAt,
		Status: loop.status, CommittedSteps: loop.committedSteps, EventSequence: loop.eventSequence,
		Limits: loop.limits, TreeLimits: loop.treeLimits,
		Budget: loop.budget, ReservedBudget: loop.reservedBudget,
		Capabilities: loop.capabilities, Usage: loop.usage,
		LastStableState: loop.lastStableState, Mailbox: loop.mailbox.snapshot(),
		PauseReason: loop.pauseReason, PendingControl: loop.pendingControl.wire(),
	}
	if loop.controller.childRequestDigest.Valid() {
		digest := loop.controller.childRequestDigest
		wire.ChildRequestDigest = &digest
	}
	if !loop.finishedAt.IsZero() {
		finishedAt := loop.finishedAt
		wire.FinishedAt = &finishedAt
	}
	if loop.currentWaitID.Valid() {
		waitID := loop.currentWaitID
		wire.CurrentWaitID = &waitID
	}
	if loop.finalOutput.Valid() {
		output := loop.finalOutput
		wire.Output = &output
	}
	if loop.termination.Valid() {
		termination := loop.termination
		wire.Termination = &termination
	}
	if loop.prepared != nil {
		prepared := clonePreparedStep(loop.prepared.wire)
		wire.Prepared = &prepared
	}
	return newSnapshot(wire)
}

func (loop *processLoop) result() Result {
	return Result{
		processID: loop.controller.processID, startedAt: loop.startedAt,
		finishedAt: loop.finishedAt, output: loop.finalOutput,
		termination: loop.termination, usage: loop.usage,
	}
}

func (control pendingControl) wire() pendingControlWire {
	wire := pendingControlWire{PauseReason: control.pauseReason}
	if control.kill.valid() {
		wire.KillReason = control.kill.reason
	}
	if control.deadline.valid() {
		wire.DeadlineOwner = control.deadline.owner.String()
		wire.DeadlineReason = control.deadline.reason
	}
	if control.cancellation.valid() {
		wire.CancellationOwner = control.cancellation.owner.String()
		wire.CancellationReason = control.cancellation.reason
	}
	return wire
}

func pendingControlFromWire(wire pendingControlWire) (pendingControl, error) {
	if err := validatePendingControlWire(wire); err != nil {
		return pendingControl{}, err
	}
	control := pendingControl{pauseReason: wire.PauseReason}
	if wire.KillReason != "" {
		control.kill, _ = newKillIntent(wire.KillReason)
	}
	if wire.DeadlineOwner != "" {
		owner, _ := parseDeadlineOwner(wire.DeadlineOwner)
		control.deadline, _ = newDeadlineIntent(owner, wire.DeadlineReason)
	}
	if wire.CancellationOwner != "" {
		owner, _ := parseCancellationOwner(wire.CancellationOwner)
		control.cancellation, _ = newCancellationIntent(owner, wire.CancellationReason)
	}
	return control, nil
}

func clonePreparedStep(value preparedStepWire) preparedStepWire {
	clone := value
	clone.Effects = make([]preparedEffectWire, len(value.Effects))
	for index, effect := range value.Effects {
		clone.Effects[index] = preparedEffectWire{ID: effect.ID, Effect: effect.Effect.clone()}
		if effect.WaitID != nil {
			waitID := *effect.WaitID
			clone.Effects[index].WaitID = &waitID
		}
		if effect.Settlement != nil {
			settlement := *effect.Settlement
			clone.Effects[index].Settlement = &settlement
		}
	}
	return clone
}
