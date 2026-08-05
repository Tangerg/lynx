package agent2

import "fmt"

func restoreProcessRuntime(
	engine *Engine,
	controller *processController,
	deployment Deployment,
	execution Execution,
	mailbox signalMailbox,
	wire processSnapshotWire,
) (*processRuntime, error) {
	runtime := &processRuntime{
		engine: engine, controller: controller, deployment: deployment, execution: execution,
		startedAt: wire.StartedAt, status: wire.Status, committedSteps: wire.CommittedSteps,
		eventSequence: wire.EventSequence, lastStable: wire.LastStable, mailbox: mailbox, restored: true,
		pauseReason: wire.PauseReason, limits: wire.Limits, treeLimits: wire.TreeLimits,
		budget: wire.Budget, reservedBudget: wire.ReservedBudget,
		capabilities: wire.Capabilities, usage: wire.Usage,
	}
	if wire.ChildRequestDigest != nil {
		controller.childRequestDigest = *wire.ChildRequestDigest
	}
	if wire.FinishedAt != nil {
		runtime.finishedAt = *wire.FinishedAt
	}
	if wire.CurrentWaitID != nil {
		runtime.currentWaitID = *wire.CurrentWaitID
	}
	if wire.Output != nil {
		runtime.output = *wire.Output
	}
	if wire.Termination != nil {
		runtime.termination = *wire.Termination
	}
	control, err := pendingControlFromWire(wire.PendingControl)
	if err != nil {
		return nil, fmt.Errorf("%w: pending control: %w", ErrInvalidSnapshot, err)
	}
	runtime.control = control
	if wire.Prepared != nil {
		copy := clonePreparedStep(*wire.Prepared)
		runtime.prepared = &preparedStep{wire: copy, acknowledged: true, fromSnapshot: true}
	}
	controller.updateView(runtime.status, runtime.currentWaitID, runtime.usage)
	return runtime, nil
}

func (runtime *processRuntime) capture() (Snapshot, error) {
	wire := processSnapshotWire{
		SchemaVersion: processSnapshotSchemaVersion, ProcessID: runtime.controller.id,
		Relation:   runtime.controller.relation.wire(),
		Deployment: runtime.deployment.Reference(), StartedAt: runtime.startedAt,
		Status: runtime.status, CommittedSteps: runtime.committedSteps, EventSequence: runtime.eventSequence,
		Limits: runtime.limits, TreeLimits: runtime.treeLimits,
		Budget: runtime.budget, ReservedBudget: runtime.reservedBudget,
		Capabilities: runtime.capabilities, Usage: runtime.usage,
		LastStable: runtime.lastStable, Mailbox: runtime.mailbox.snapshot(),
		PauseReason: runtime.pauseReason, PendingControl: runtime.control.wire(),
	}
	if runtime.controller.childRequestDigest.Valid() {
		digest := runtime.controller.childRequestDigest
		wire.ChildRequestDigest = &digest
	}
	if !runtime.finishedAt.IsZero() {
		finishedAt := runtime.finishedAt
		wire.FinishedAt = &finishedAt
	}
	if runtime.currentWaitID.Valid() {
		waitID := runtime.currentWaitID
		wire.CurrentWaitID = &waitID
	}
	if runtime.output.Valid() {
		output := runtime.output
		wire.Output = &output
	}
	if runtime.termination.Valid() {
		termination := runtime.termination
		wire.Termination = &termination
	}
	if runtime.prepared != nil {
		prepared := clonePreparedStep(runtime.prepared.wire)
		wire.Prepared = &prepared
	}
	return newSnapshot(wire)
}

func (runtime *processRuntime) result() Result {
	return Result{
		processID: runtime.controller.id, startedAt: runtime.startedAt,
		finishedAt: runtime.finishedAt, output: runtime.output,
		termination: runtime.termination, usage: runtime.usage,
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
