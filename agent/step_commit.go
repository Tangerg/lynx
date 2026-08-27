package agent

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

func (p *processLoop) prepareNextStep(ctx context.Context) {
	if !resourceQuantitiesFit(p.limits.MaxSteps, p.committedSteps, 1) ||
		!resourceQuantitiesFit(p.budget.Steps, p.committedSteps, p.reservedBudget.Steps, 1) {
		p.fail(FailureKindExecution, "engine.limit.steps", ErrResourceLimitExceeded)
		return
	}
	sequence := p.committedSteps + 1
	p.publishEvent(ctx, EventStepStarted, EventPhaseAttempt, sequence, EffectID{}, emptyEventPayload())
	stepStartedAt := time.Now()
	signals := p.mailbox.pending()
	transition, err := stepExecution(ctx, p.execution, signals)
	stepStatus := StepStatusSucceeded
	if err != nil {
		stepStatus = StepStatusFailed
	}
	stepPayload, _ := json.Marshal(stepFinishedEventPayload{
		StepStatus: stepStatus, DurationMS: time.Since(stepStartedAt).Milliseconds(),
	})
	p.publishEvent(ctx, EventStepFinished, EventPhaseAttempt, sequence, EffectID{}, stepPayload)
	if err != nil {
		p.observeHostError(ctx)
		p.discardExecution()
		p.fail(failureKindForError(err), "execution.step.failed", err)
		return
	}
	if !transition.Valid() || uint64(transition.ConsumedSignals()) > uint64(len(signals)) {
		p.discardExecution()
		p.fail(FailureKindContract, "execution.transition.invalid", ErrInvalidTransition)
		return
	}
	for _, effect := range transition.Effects() {
		if !p.capabilities.Allows(effect.RequiredCapabilities()) {
			p.discardExecution()
			p.fail(FailureKindContract, "engine.capability.denied", ErrInvalidCapability)
			return
		}
	}
	effectCount := uint64(len(transition.Effects()))
	if !resourceQuantitiesFit(p.limits.MaxEffects, p.usage.PreparedEffects, effectCount) ||
		!resourceQuantitiesFit(
			p.budget.Effects, p.usage.PreparedEffects, p.reservedBudget.Effects, effectCount,
		) {
		p.discardExecution()
		p.fail(FailureKindExecution, "engine.limit.effects", ErrResourceLimitExceeded)
		return
	}
	remainingPending := p.mailbox.pendingCount() - uint64(transition.ConsumedSignals())
	if !resourceQuantitiesFit(p.limits.MaxSignals, p.usage.AcceptedSignals, effectCount) ||
		!resourceQuantitiesFit(p.limits.MaxPendingSignals, remainingPending, effectCount) ||
		!resourceQuantitiesFit(
			p.budget.Signals, p.usage.AcceptedSignals, p.reservedBudget.Signals, effectCount,
		) {
		p.discardExecution()
		p.fail(FailureKindExecution, "engine.limit.signals", ErrResourceLimitExceeded)
		return
	}
	if output, completes := transition.Output(); completes {
		if validateOutputErr := p.deployment.Descriptor().ValidateOutput(output); validateOutputErr != nil {
			p.discardExecution()
			p.fail(FailureKindContract, "execution.output.invalid", validateOutputErr)
			return
		}
	}
	candidateState, err := captureExecution(p.execution)
	if err != nil {
		p.discardExecution()
		p.fail(failureKindForError(err), "execution.snapshot.failed", err)
		return
	}
	candidate, err := restoreExecution(p.deployment.Definition(), candidateState)
	if err != nil {
		p.discardExecution()
		p.fail(failureKindForError(err), "execution.snapshot.unrestorable", err)
		return
	}
	digest, err := executionStateDigest(p.lastStableState)
	if err != nil {
		p.discardExecution()
		p.fail(FailureKindContract, "engine.last_stable.invalid", err)
		return
	}
	wire := preparedStepWire{
		StepSequence: sequence, LastStableDigest: digest, CandidateState: candidateState,
		SignalCursor: p.mailbox.committedSignalCursor() + uint64(transition.ConsumedSignals()),
		Transition:   transition,
	}
	for index, effect := range transition.Effects() {
		wire.Effects = append(wire.Effects, preparedEffectWire{
			ID: deriveEffectID(p.controller.processID, sequence, index), Effect: effect,
		})
	}
	p.prepared = &preparedStep{
		wire: wire, candidate: candidate, acknowledged: len(wire.Effects) == 0,
	}
	p.usage.PreparedEffects += effectCount
	p.updateView()
	p.publishEvent(ctx, EventStepPrepared, EventPhaseAttempt, sequence, EffectID{}, emptyEventPayload())
}

func (p *processLoop) acknowledgePrepared(ctx context.Context) bool {
	if p.engine.acknowledger == nil {
		p.prepared.acknowledged = true
		return true
	}
	snapshot, err := p.capture()
	if err == nil {
		err = acknowledgePreparedStep(ctx, p.engine.acknowledger, snapshot)
	}
	if err == nil {
		p.prepared.acknowledged = true
		return true
	}
	p.observeHostError(ctx)
	p.discardPrepared()
	p.fail(FailureKindExternal, "engine.prepared_acknowledgment.failed", err)
	return false
}

func (p *processLoop) finalizePrepared(ctx context.Context) error {
	finalization, err := newPreparedStepFinalization(p)
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

func (p *preparedStepFinalization) prepare() error {
	for _, record := range p.prepared.wire.Effects {
		if err := p.applySettlement(record); err != nil {
			return err
		}
	}
	if err := p.enqueueImmediateChildSignals(); err != nil {
		return err
	}
	return p.prepareTransition()
}

func (p *preparedStepFinalization) applySettlement(record preparedEffectWire) error {
	if record.Settlement == nil || record.Settlement.Status() == SettlementStatusUnknown {
		return errors.New("effect batch is not definitely settled")
	}
	waitID, err := p.registerFrameworkEffect(record)
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
		return p.mailbox.enqueueWaitOpened(signal)
	}
	accepted, err := p.mailbox.enqueue(StatusRunning, signal)
	if err != nil || !accepted {
		return errors.Join(err, errors.New("internal settlement Signal was not accepted"))
	}
	return nil
}

func (p *preparedStepFinalization) registerFrameworkEffect(record preparedEffectWire) (WaitID, error) {
	if record.Effect.Target() != EffectTargetFramework {
		return WaitID{}, nil
	}
	operation, err := decodeFrameworkEffectOperation(record.Effect.Payload())
	if err != nil {
		return WaitID{}, errors.New("invalid prepared framework Effect")
	}
	switch operation {
	case frameworkEffectWait:
		key, _, err := decodeWaitRequest(record.Effect)
		if err != nil || record.WaitID == nil {
			return WaitID{}, errors.New("invalid prepared wait Effect")
		}
		if err := p.mailbox.registerWait(key, *record.WaitID, true); err != nil {
			return WaitID{}, err
		}
		return *record.WaitID, nil
	case frameworkEffectStartChild:
		if record.WaitID != nil {
			return WaitID{}, errors.New("child-start Effect unexpectedly contains a WaitID")
		}
		return WaitID{}, nil
	case frameworkEffectWaitChildren:
		return p.registerChildWait(record)
	default:
		return WaitID{}, errors.New("unsupported prepared framework Effect")
	}
}

func (p *preparedStepFinalization) registerChildWait(record preparedEffectWire) (WaitID, error) {
	spec, err := decodeChildWaitEffect(record.Effect.Payload())
	if err != nil || record.WaitID == nil {
		return WaitID{}, errors.New("invalid child-wait Effect")
	}
	waitID := *record.WaitID
	if registerWaitErr := p.mailbox.registerWait(spec.Key, waitID, false); registerWaitErr != nil {
		return WaitID{}, registerWaitErr
	}
	immediateSignal, immediatelySatisfied, err := p.loop.engine.registerChildWait(
		p.loop.controller.processID, waitID, spec,
	)
	if err != nil {
		return WaitID{}, err
	}
	p.registeredChildWaits = append(p.registeredChildWaits, waitID)
	if immediatelySatisfied {
		p.immediateChildSignals = append(p.immediateChildSignals, immediateSignal)
	}
	return waitID, nil
}

func (p *preparedStepFinalization) enqueueImmediateChildSignals() error {
	preparedSignals := uint64(len(p.prepared.wire.Effects))
	for index, signal := range p.immediateChildSignals {
		acceptedSignals := uint64(index) + 1
		if !resourceQuantitiesFit(
			p.loop.limits.MaxSignals,
			p.loop.usage.AcceptedSignals, preparedSignals, acceptedSignals,
		) || !resourceQuantitiesFit(
			p.loop.limits.MaxPendingSignals, p.mailbox.pendingCount(), 1,
		) || !resourceQuantitiesFit(
			p.loop.budget.Signals,
			p.loop.usage.AcceptedSignals, p.loop.reservedBudget.Signals,
			preparedSignals, acceptedSignals,
		) {
			return ErrResourceLimitExceeded
		}
		accepted, err := p.mailbox.enqueueChildCompletion(StatusRunning, signal)
		if err != nil || !accepted {
			return errors.Join(err, errors.New("immediate child completion Signal was not accepted"))
		}
	}
	return nil
}

func (p *preparedStepFinalization) prepareTransition() error {
	transition := p.prepared.wire.Transition
	switch transition.Kind() {
	case TransitionKindContinue:
		p.transition.status = StatusRunning
	case TransitionKindWait:
		return p.prepareWaitTransition(transition)
	case TransitionKindPause:
		p.transition.status = StatusPaused
		p.transition.pauseReason, _ = transition.Reason()
	case TransitionKindComplete:
		p.transition.finalOutput, _ = transition.Output()
		p.prepareTermination(completedOutcome())
	case TransitionKindFail:
		failure, _ := transition.Failure()
		outcome, err := failedOutcome(failure)
		if err != nil {
			return err
		}
		p.prepareTermination(outcome)
	default:
		return ErrInvalidTransition
	}
	return nil
}

func (p *preparedStepFinalization) prepareWaitTransition(transition Transition) error {
	waitID, _ := transition.WaitID()
	shouldWait, err := p.mailbox.enterWait(waitID)
	if err != nil {
		return err
	}
	if shouldWait {
		p.transition.status = StatusWaiting
		p.transition.currentWaitID = waitID
	} else {
		p.transition.status = StatusRunning
	}
	return nil
}

func (p *preparedStepFinalization) prepareTermination(outcome stepOutcome) {
	p.transition.termination = p.loop.resolveStepTermination(outcome)
	p.transition.status = p.transition.termination.Status()
	p.transition.finishedAt = time.Now().Round(0).UTC()
	p.transition.closedChildWaits = p.mailbox.closeAllWaits()
}

func (p *preparedStepFinalization) commit(ctx context.Context) error {
	execution := p.prepared.candidate
	if execution == nil {
		var err error
		execution, err = restoreExecution(
			p.loop.deployment.Definition(), p.prepared.wire.CandidateState,
		)
		if err != nil {
			return err
		}
	}
	loop := p.loop
	loop.execution = execution
	loop.lastStableState = p.prepared.wire.CandidateState
	loop.mailbox = p.mailbox
	loop.committedSteps = p.prepared.wire.StepSequence
	loop.usage.CommittedSteps = loop.committedSteps
	loop.usage.AcceptedSignals += uint64(len(p.prepared.wire.Effects))
	loop.usage.AcceptedSignals += uint64(len(p.immediateChildSignals))
	loop.prepared = nil
	loop.status = p.transition.status
	loop.currentWaitID = p.transition.currentWaitID
	loop.pauseReason = p.transition.pauseReason
	loop.finalOutput = p.transition.finalOutput
	if p.transition.termination.Valid() {
		loop.termination = p.transition.termination
		loop.finishedAt = p.transition.finishedAt
		loop.pendingControl = pendingControl{}
	}
	loop.updateView()
	payload, _ := json.Marshal(stepCommittedEventPayload{ProcessStatus: loop.status})
	loop.publishEvent(ctx, EventStepCommitted, EventPhaseCommitted, loop.committedSteps, EffectID{}, payload)
	for _, waitID := range p.consumedChildWaits {
		loop.engine.unregisterChildWait(waitID)
	}
	for _, waitID := range p.transition.closedChildWaits {
		loop.engine.unregisterChildWait(waitID)
	}
	p.committed = true
	return nil
}

func (p *preparedStepFinalization) rollback() {
	if p == nil || p.committed {
		return
	}
	for _, waitID := range p.registeredChildWaits {
		p.loop.engine.unregisterChildWait(waitID)
	}
}

func (p *processLoop) discardPrepared() {
	p.prepared = nil
	p.discardExecution()
}

func (p *processLoop) discardExecution() {
	execution, err := restoreExecution(p.deployment.Definition(), p.lastStableState)
	if err == nil {
		p.execution = execution
	} else {
		p.execution = nil
	}
}

func (p *processLoop) fail(kind FailureKind, code string, err error) {
	failure, failureErr := failureFromError(kind, code, err)
	if failureErr != nil {
		failure, _ = NewFailure(FailureKindContract, "engine.failure.invalid", "Engine could not construct a valid failure")
	}
	outcome, _ := failedOutcome(failure)
	p.commitTermination(outcome)
}

func (p *processLoop) commitTermination(outcome stepOutcome) {
	termination := p.resolveStepTermination(outcome)
	p.termination = termination
	p.status = termination.Status()
	p.finishedAt = time.Now().Round(0).UTC()
	p.currentWaitID = WaitID{}
	p.pauseReason = ""
	p.pendingControl = pendingControl{}
	for _, waitID := range p.mailbox.closeAllWaits() {
		p.engine.unregisterChildWait(waitID)
	}
	p.updateView()
}

func (p *processLoop) resolveStepTermination(outcome stepOutcome) Termination {
	termination, err := resolveTermination(terminationFacts{
		kill: p.pendingControl.kill, deadline: p.pendingControl.deadline,
		cancellation: p.pendingControl.cancellation, outcome: outcome,
	})
	if err != nil {
		failure, _ := NewFailure(FailureKindContract, "engine.termination.invalid", err.Error())
		termination = terminationForFailure(failure)
	}
	return termination
}

func (p *processLoop) observeHostError(ctx context.Context) {
	if err := ctx.Err(); err != nil {
		p.recordHostTermination(err)
	}
}
