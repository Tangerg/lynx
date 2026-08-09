package agent

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

func (loop *processLoop) prepareNextStep(ctx context.Context) {
	if !resourceQuantitiesFit(loop.limits.MaxSteps, loop.committedSteps, 1) ||
		!resourceQuantitiesFit(loop.budget.Steps, loop.committedSteps, loop.reservedBudget.Steps, 1) {
		loop.fail(FailureKindExecution, "engine.limit.steps", ErrResourceLimitExceeded)
		return
	}
	sequence := loop.committedSteps + 1
	loop.publishEvent(ctx, EventStepStarted, EventPhaseAttempt, sequence, EffectID{}, emptyEventPayload())
	stepStartedAt := time.Now()
	signals := loop.mailbox.pending()
	transition, err := stepExecution(ctx, loop.execution, signals)
	stepStatus := "succeeded"
	if err != nil {
		stepStatus = "failed"
	}
	stepPayload, _ := json.Marshal(stepFinishedEventPayload{
		StepStatus: stepStatus, DurationMS: time.Since(stepStartedAt).Milliseconds(),
	})
	loop.publishEvent(ctx, EventStepFinished, EventPhaseAttempt, sequence, EffectID{}, stepPayload)
	if err != nil {
		loop.observeHostError(ctx)
		loop.discardExecution()
		loop.fail(failureKindForError(err), "execution.step.failed", err)
		return
	}
	if !transition.Valid() || uint64(transition.ConsumedSignals()) > uint64(len(signals)) {
		loop.discardExecution()
		loop.fail(FailureKindContract, "execution.transition.invalid", ErrInvalidTransition)
		return
	}
	for _, effect := range transition.Effects() {
		if !loop.capabilities.Allows(effect.RequiredCapabilities()) {
			loop.discardExecution()
			loop.fail(FailureKindContract, "engine.capability.denied", ErrInvalidCapability)
			return
		}
	}
	effectCount := uint64(len(transition.Effects()))
	if !resourceQuantitiesFit(loop.limits.MaxEffects, loop.usage.PreparedEffects, effectCount) ||
		!resourceQuantitiesFit(
			loop.budget.Effects, loop.usage.PreparedEffects, loop.reservedBudget.Effects, effectCount,
		) {
		loop.discardExecution()
		loop.fail(FailureKindExecution, "engine.limit.effects", ErrResourceLimitExceeded)
		return
	}
	remainingPending := loop.mailbox.pendingCount() - uint64(transition.ConsumedSignals())
	if !resourceQuantitiesFit(loop.limits.MaxSignals, loop.usage.AcceptedSignals, effectCount) ||
		!resourceQuantitiesFit(loop.limits.MaxPendingSignals, remainingPending, effectCount) ||
		!resourceQuantitiesFit(
			loop.budget.Signals, loop.usage.AcceptedSignals, loop.reservedBudget.Signals, effectCount,
		) {
		loop.discardExecution()
		loop.fail(FailureKindExecution, "engine.limit.signals", ErrResourceLimitExceeded)
		return
	}
	if output, completes := transition.Output(); completes {
		if err := loop.deployment.Descriptor().ValidateOutput(output); err != nil {
			loop.discardExecution()
			loop.fail(FailureKindContract, "execution.output.invalid", err)
			return
		}
	}
	candidateState, err := captureExecution(loop.execution)
	if err != nil {
		loop.discardExecution()
		loop.fail(failureKindForError(err), "execution.snapshot.failed", err)
		return
	}
	candidate, err := restoreExecution(loop.deployment.Definition(), candidateState)
	if err != nil {
		loop.discardExecution()
		loop.fail(failureKindForError(err), "execution.snapshot.unrestorable", err)
		return
	}
	digest, err := executionStateDigest(loop.lastStableState)
	if err != nil {
		loop.discardExecution()
		loop.fail(FailureKindContract, "engine.last_stable.invalid", err)
		return
	}
	wire := preparedStepWire{
		StepSequence: sequence, LastStableDigest: digest, CandidateState: candidateState,
		SignalCursor: loop.mailbox.committedSignalCursor() + uint64(transition.ConsumedSignals()),
		Transition:   transition,
	}
	for index, effect := range transition.Effects() {
		wire.Effects = append(wire.Effects, preparedEffectWire{
			ID: deriveEffectID(loop.controller.processID, sequence, index), Effect: effect,
		})
	}
	loop.prepared = &preparedStep{
		wire: wire, candidate: candidate, acknowledged: len(wire.Effects) == 0,
	}
	loop.usage.PreparedEffects += effectCount
	loop.updateView()
	loop.publishEvent(ctx, EventStepPrepared, EventPhaseAttempt, sequence, EffectID{}, emptyEventPayload())
}

func (loop *processLoop) acknowledgePrepared(ctx context.Context) bool {
	if loop.engine.acknowledger == nil {
		loop.prepared.acknowledged = true
		return true
	}
	snapshot, err := loop.capture()
	if err == nil {
		err = acknowledgePreparedStep(ctx, loop.engine.acknowledger, snapshot)
	}
	if err == nil {
		loop.prepared.acknowledged = true
		return true
	}
	loop.observeHostError(ctx)
	loop.discardPrepared()
	loop.fail(FailureKindExternal, "engine.prepared_acknowledgment.failed", err)
	return false
}

func (loop *processLoop) finalizePrepared(ctx context.Context) error {
	finalization, err := newPreparedStepFinalization(loop)
	if err != nil {
		return err
	}
	defer finalization.rollback()
	if err := finalization.prepare(); err != nil {
		return err
	}
	return finalization.commit(ctx)
}

type preparedStepFinalization struct {
	loop                  *processLoop
	prepared              *preparedStep
	mailbox               signalMailbox
	consumedChildWaits    []WaitID
	registeredChildWaits  []WaitID
	immediateChildSignals []Signal
	transition            preparedTransitionState
	committed             bool
}

type preparedTransitionState struct {
	status           Status
	currentWaitID    WaitID
	pauseReason      string
	finalOutput      Output
	termination      Termination
	finishedAt       time.Time
	closedChildWaits []WaitID
}

func newPreparedStepFinalization(loop *processLoop) (*preparedStepFinalization, error) {
	mailbox, err := restoreSignalMailbox(loop.mailbox.snapshot())
	if err != nil {
		return nil, err
	}
	consumedChildWaits, err := mailbox.consumedChildWaitIDs(loop.prepared.wire.Transition.ConsumedSignals())
	if err != nil {
		return nil, err
	}
	if err := mailbox.commit(loop.prepared.wire.Transition.ConsumedSignals()); err != nil {
		return nil, err
	}
	return &preparedStepFinalization{
		loop: loop, prepared: loop.prepared, mailbox: mailbox,
		consumedChildWaits: consumedChildWaits,
	}, nil
}

func (finalization *preparedStepFinalization) prepare() error {
	for _, record := range finalization.prepared.wire.Effects {
		if err := finalization.applySettlement(record); err != nil {
			return err
		}
	}
	if err := finalization.enqueueImmediateChildSignals(); err != nil {
		return err
	}
	return finalization.prepareTransition()
}

func (finalization *preparedStepFinalization) applySettlement(record preparedEffectWire) error {
	if record.Settlement == nil || record.Settlement.Status() == SettlementStatusUnknown {
		return errors.New("effect batch is not definitely settled")
	}
	waitID, err := finalization.registerFrameworkEffect(record)
	if err != nil {
		return err
	}
	signal, err := newSignal(
		deriveSettlementSignalID(record.ID), waitID, time.Now(), record.Settlement.Payload(),
	)
	if err != nil {
		return err
	}
	if waitID.Valid() {
		return finalization.mailbox.enqueueWaitOpened(signal)
	}
	accepted, err := finalization.mailbox.enqueue(StatusRunning, signal)
	if err != nil || !accepted {
		return errors.Join(err, errors.New("internal settlement Signal was not accepted"))
	}
	return nil
}

func (finalization *preparedStepFinalization) registerFrameworkEffect(record preparedEffectWire) (WaitID, error) {
	if record.Effect.Target() != EffectTargetFramework {
		return WaitID{}, nil
	}
	operation, err := frameworkEffectOperation(record.Effect.Payload())
	if err != nil {
		return WaitID{}, errors.New("invalid prepared framework Effect")
	}
	switch operation {
	case frameworkEffectWait:
		key, _, err := decodeWaitRequest(record.Effect)
		if err != nil || record.WaitID == nil {
			return WaitID{}, errors.New("invalid prepared wait Effect")
		}
		if err := finalization.mailbox.registerWait(key, *record.WaitID, true); err != nil {
			return WaitID{}, err
		}
		return *record.WaitID, nil
	case frameworkEffectStartChild:
		if record.WaitID != nil {
			return WaitID{}, errors.New("child-start Effect unexpectedly contains a WaitID")
		}
		return WaitID{}, nil
	case frameworkEffectWaitChildren:
		return finalization.registerChildWait(record)
	default:
		return WaitID{}, errors.New("unsupported prepared framework Effect")
	}
}

func (finalization *preparedStepFinalization) registerChildWait(record preparedEffectWire) (WaitID, error) {
	spec, err := decodeChildWaitEffect(record.Effect.Payload())
	if err != nil || record.WaitID == nil {
		return WaitID{}, errors.New("invalid child-wait Effect")
	}
	waitID := *record.WaitID
	if err := finalization.mailbox.registerWait(spec.Key, waitID, false); err != nil {
		return WaitID{}, err
	}
	immediateSignal, immediatelySatisfied, err := finalization.loop.engine.registerChildWait(
		finalization.loop.controller.processID, waitID, spec,
	)
	if err != nil {
		return WaitID{}, err
	}
	finalization.registeredChildWaits = append(finalization.registeredChildWaits, waitID)
	if immediatelySatisfied {
		finalization.immediateChildSignals = append(finalization.immediateChildSignals, immediateSignal)
	}
	return waitID, nil
}

func (finalization *preparedStepFinalization) enqueueImmediateChildSignals() error {
	preparedSignals := uint64(len(finalization.prepared.wire.Effects))
	for index, signal := range finalization.immediateChildSignals {
		acceptedSignals := uint64(index) + 1
		if !resourceQuantitiesFit(
			finalization.loop.limits.MaxSignals,
			finalization.loop.usage.AcceptedSignals, preparedSignals, acceptedSignals,
		) || !resourceQuantitiesFit(
			finalization.loop.limits.MaxPendingSignals, finalization.mailbox.pendingCount(), 1,
		) || !resourceQuantitiesFit(
			finalization.loop.budget.Signals,
			finalization.loop.usage.AcceptedSignals, finalization.loop.reservedBudget.Signals,
			preparedSignals, acceptedSignals,
		) {
			return ErrResourceLimitExceeded
		}
		accepted, err := finalization.mailbox.enqueueChildCompletion(StatusRunning, signal)
		if err != nil || !accepted {
			return errors.Join(err, errors.New("immediate child completion Signal was not accepted"))
		}
	}
	return nil
}

func (finalization *preparedStepFinalization) prepareTransition() error {
	transition := finalization.prepared.wire.Transition
	switch transition.Kind() {
	case TransitionKindContinue:
		finalization.transition.status = StatusRunning
	case TransitionKindWait:
		return finalization.prepareWaitTransition(transition)
	case TransitionKindPause:
		finalization.transition.status = StatusPaused
		finalization.transition.pauseReason, _ = transition.Reason()
	case TransitionKindComplete:
		finalization.transition.finalOutput, _ = transition.Output()
		finalization.prepareTermination(completedOutcome())
	case TransitionKindFail:
		failure, _ := transition.Failure()
		outcome, err := failedOutcome(failure)
		if err != nil {
			return err
		}
		finalization.prepareTermination(outcome)
	default:
		return ErrInvalidTransition
	}
	return nil
}

func (finalization *preparedStepFinalization) prepareWaitTransition(transition Transition) error {
	waitID, _ := transition.WaitID()
	shouldWait, err := finalization.mailbox.enterWait(waitID)
	if err != nil {
		return err
	}
	if shouldWait {
		finalization.transition.status = StatusWaiting
		finalization.transition.currentWaitID = waitID
	} else {
		finalization.transition.status = StatusRunning
	}
	return nil
}

func (finalization *preparedStepFinalization) prepareTermination(outcome stepOutcome) {
	finalization.transition.termination = finalization.loop.resolveStepTermination(outcome)
	finalization.transition.status = finalization.transition.termination.Status()
	finalization.transition.finishedAt = time.Now().Round(0).UTC()
	finalization.transition.closedChildWaits = finalization.mailbox.closeAllWaits()
}

func (finalization *preparedStepFinalization) commit(ctx context.Context) error {
	execution := finalization.prepared.candidate
	if execution == nil {
		var err error
		execution, err = restoreExecution(
			finalization.loop.deployment.Definition(), finalization.prepared.wire.CandidateState,
		)
		if err != nil {
			return err
		}
	}
	loop := finalization.loop
	loop.execution = execution
	loop.lastStableState = finalization.prepared.wire.CandidateState
	loop.mailbox = finalization.mailbox
	loop.committedSteps = finalization.prepared.wire.StepSequence
	loop.usage.CommittedSteps = loop.committedSteps
	loop.usage.AcceptedSignals += uint64(len(finalization.prepared.wire.Effects))
	loop.usage.AcceptedSignals += uint64(len(finalization.immediateChildSignals))
	loop.prepared = nil
	loop.status = finalization.transition.status
	loop.currentWaitID = finalization.transition.currentWaitID
	loop.pauseReason = finalization.transition.pauseReason
	loop.finalOutput = finalization.transition.finalOutput
	if finalization.transition.termination.Valid() {
		loop.termination = finalization.transition.termination
		loop.finishedAt = finalization.transition.finishedAt
		loop.pendingControl = pendingControl{}
	}
	loop.updateView()
	payload, _ := json.Marshal(stepCommittedEventPayload{ProcessStatus: loop.status.String()})
	loop.publishEvent(ctx, EventStepCommitted, EventPhaseCommitted, loop.committedSteps, EffectID{}, payload)
	for _, waitID := range finalization.consumedChildWaits {
		loop.engine.unregisterChildWait(waitID)
	}
	for _, waitID := range finalization.transition.closedChildWaits {
		loop.engine.unregisterChildWait(waitID)
	}
	finalization.committed = true
	return nil
}

func (finalization *preparedStepFinalization) rollback() {
	if finalization == nil || finalization.committed {
		return
	}
	for _, waitID := range finalization.registeredChildWaits {
		finalization.loop.engine.unregisterChildWait(waitID)
	}
}

func (loop *processLoop) discardPrepared() {
	loop.prepared = nil
	loop.discardExecution()
}

func (loop *processLoop) discardExecution() {
	execution, err := restoreExecution(loop.deployment.Definition(), loop.lastStableState)
	if err == nil {
		loop.execution = execution
	} else {
		loop.execution = nil
	}
}

func (loop *processLoop) fail(kind FailureKind, code string, err error) {
	failure, failureErr := failureFromError(kind, code, err)
	if failureErr != nil {
		failure, _ = NewFailure(FailureKindContract, "engine.failure.invalid", "Engine could not construct a valid failure")
	}
	outcome, _ := failedOutcome(failure)
	loop.commitTermination(outcome)
}

func (loop *processLoop) commitTermination(outcome stepOutcome) {
	termination := loop.resolveStepTermination(outcome)
	loop.termination = termination
	loop.status = termination.Status()
	loop.finishedAt = time.Now().Round(0).UTC()
	loop.currentWaitID = WaitID{}
	loop.pauseReason = ""
	loop.pendingControl = pendingControl{}
	for _, waitID := range loop.mailbox.closeAllWaits() {
		loop.engine.unregisterChildWait(waitID)
	}
	loop.updateView()
}

func (loop *processLoop) resolveStepTermination(outcome stepOutcome) Termination {
	termination, err := resolveTermination(terminationFacts{
		kill: loop.pendingControl.kill, deadline: loop.pendingControl.deadline,
		cancellation: loop.pendingControl.cancellation, outcome: outcome,
	})
	if err != nil {
		failure, _ := NewFailure(FailureKindContract, "engine.termination.invalid", err.Error())
		termination = terminationForFailure(failure)
	}
	return termination
}

func (loop *processLoop) observeHostError(ctx context.Context) {
	if err := ctx.Err(); err != nil {
		loop.recordHostTermination(err)
	}
}
