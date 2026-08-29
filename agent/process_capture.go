package agent

import (
	"encoding/json"
	"fmt"
)

func prepareRestoredProcess(
	engine *Engine,
	deployment Deployment,
	snapshot Snapshot,
) (*processController, *processState, processSnapshotWire, error) {
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
		if validateOutputErr := deployment.Descriptor().ValidateOutput(*wire.Output); validateOutputErr != nil {
			return nil, nil, processSnapshotWire{}, fmt.Errorf(
				"%w: output schema: %w", ErrInvalidSnapshot, validateOutputErr,
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
) (*processState, error) {
	loop := &processState{
		engine: engine, controller: controller, deployment: deployment, execution: execution,
		startedAt: wire.StartedAt, status: wire.Status, committedSteps: wire.CommittedSteps,
		processEventSequence: wire.ProcessEventSequence, lastStableState: wire.LastStableState, mailbox: mailbox, restored: true,
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
		preparedWire := clonePreparedStep(*wire.Prepared)
		for index := range preparedWire.Effects {
			record := &preparedWire.Effects[index]
			if record.Settlement != nil || record.Effect.Target() != EffectTargetDispatcher {
				continue
			}
			if dispatcherReplayPolicy(deployment.effectDispatcher(), record.Effect) == ReplayPolicySameIdentity {
				continue
			}
			settlement, _ := NewSettlement(
				record.ID, SettlementStatusUnknown, json.RawMessage(nullJSON),
			)
			record.Settlement = &settlement
		}
		loop.prepared = &preparedStep{wire: preparedWire, acknowledged: true}
	}
	controller.updateView(loop.status, loop.currentWaitID, loop.usage)
	return loop, nil
}

func (p *processState) capture() (Snapshot, error) {
	wire := processSnapshotWire{
		SchemaVersion: processSnapshotSchemaVersion, ProcessID: p.controller.processID,
		Relation:      p.controller.relation.wire(),
		DeploymentRef: p.deployment.DeploymentRef(), StartedAt: p.startedAt,
		Status: p.status, CommittedSteps: p.committedSteps, ProcessEventSequence: p.processEventSequence,
		Limits: p.limits, TreeLimits: p.treeLimits,
		Budget: p.budget, ReservedBudget: p.reservedBudget,
		Capabilities: p.capabilities, Usage: p.usage,
		LastStableState: p.lastStableState, Mailbox: p.mailbox.snapshot(),
		PauseReason: p.pauseReason, PendingControl: p.pendingControl.wire(),
	}
	if p.controller.childRequestDigest.Valid() {
		digest := p.controller.childRequestDigest
		wire.ChildRequestDigest = &digest
	}
	if !p.finishedAt.IsZero() {
		finishedAt := p.finishedAt
		wire.FinishedAt = &finishedAt
	}
	if p.currentWaitID.Valid() {
		waitID := p.currentWaitID
		wire.CurrentWaitID = &waitID
	}
	if p.finalOutput.Valid() {
		output := p.finalOutput
		wire.Output = &output
	}
	if p.termination.Valid() {
		termination := p.termination
		wire.Termination = &termination
	}
	if p.prepared != nil {
		prepared := clonePreparedStep(p.prepared.wire)
		wire.Prepared = &prepared
	}
	return newSnapshot(wire)
}

func (p *processState) result() Result {
	return Result{
		processID: p.controller.processID, startedAt: p.startedAt,
		finishedAt: p.finishedAt, output: p.finalOutput,
		termination: p.termination, usage: p.usage,
	}
}

func (p pendingControl) wire() pendingControlWire {
	wire := pendingControlWire{PauseReason: p.pauseReason}
	if p.kill.valid() {
		wire.KillReason = p.kill.reason
	}
	if p.deadline.valid() {
		wire.DeadlineOwner = p.deadline.owner
		wire.DeadlineReason = p.deadline.reason
	}
	if p.cancellation.valid() {
		wire.CancellationOwner = p.cancellation.owner
		wire.CancellationReason = p.cancellation.reason
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
		control.deadline, _ = newDeadlineIntent(wire.DeadlineOwner, wire.DeadlineReason)
	}
	if wire.CancellationOwner != "" {
		control.cancellation, _ = newCancellationIntent(wire.CancellationOwner, wire.CancellationReason)
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
