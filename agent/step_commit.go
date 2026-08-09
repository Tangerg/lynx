package agent

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

func (loop *processLoop) prepareNextStep(ctx context.Context) {
	if loop.committedSteps >= loop.limits.MaxSteps ||
		loop.committedSteps+1+loop.reservedBudget.Steps > loop.budget.Steps {
		loop.fail(ctx, FailureKindExecution, "engine.limit.steps", ErrResourceLimitExceeded)
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
		loop.fail(ctx, failureKindForError(err), "execution.step.failed", err)
		return
	}
	if !transition.Valid() || uint64(transition.ConsumedSignals()) > uint64(len(signals)) {
		loop.discardExecution()
		loop.fail(ctx, FailureKindContract, "execution.transition.invalid", ErrInvalidTransition)
		return
	}
	for _, effect := range transition.Effects() {
		if !loop.capabilities.Allows(effect.RequiredCapabilities()) {
			loop.discardExecution()
			loop.fail(ctx, FailureKindContract, "engine.capability.denied", ErrInvalidCapability)
			return
		}
	}
	effectCount := uint64(len(transition.Effects()))
	if loop.usage.PreparedEffects+effectCount > loop.limits.MaxEffects ||
		loop.usage.PreparedEffects+effectCount+loop.reservedBudget.Effects > loop.budget.Effects {
		loop.discardExecution()
		loop.fail(ctx, FailureKindExecution, "engine.limit.effects", ErrResourceLimitExceeded)
		return
	}
	remainingPending := loop.mailbox.pendingCount() - uint64(transition.ConsumedSignals())
	if loop.usage.AcceptedSignals+effectCount > loop.limits.MaxSignals ||
		remainingPending+effectCount > loop.limits.MaxPendingSignals ||
		loop.usage.AcceptedSignals+effectCount+loop.reservedBudget.Signals > loop.budget.Signals {
		loop.discardExecution()
		loop.fail(ctx, FailureKindExecution, "engine.limit.signals", ErrResourceLimitExceeded)
		return
	}
	if output, completes := transition.Output(); completes {
		if err := loop.deployment.Descriptor().ValidateOutput(output); err != nil {
			loop.discardExecution()
			loop.fail(ctx, FailureKindContract, "execution.output.invalid", err)
			return
		}
	}
	candidateState, err := captureExecution(loop.execution)
	if err != nil {
		loop.discardExecution()
		loop.fail(ctx, failureKindForError(err), "execution.snapshot.failed", err)
		return
	}
	candidate, err := restoreExecution(loop.deployment.Definition(), candidateState)
	if err != nil {
		loop.discardExecution()
		loop.fail(ctx, failureKindForError(err), "execution.snapshot.unrestorable", err)
		return
	}
	digest, err := executionStateDigest(loop.lastStableState)
	if err != nil {
		loop.discardExecution()
		loop.fail(ctx, FailureKindContract, "engine.last_stable.invalid", err)
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
		err = acknowledgePreparedStep(loop.engine.acknowledger, ctx, snapshot)
	}
	if err == nil {
		loop.prepared.acknowledged = true
		return true
	}
	loop.observeHostError(ctx)
	loop.discardPrepared()
	loop.fail(ctx, FailureKindExternal, "engine.prepared_acknowledgment.failed", err)
	return false
}

func (loop *processLoop) finalizePrepared(ctx context.Context) (returnErr error) {
	prepared := loop.prepared
	var immediateChildSignals []Signal
	var registeredChildWaits []WaitID
	committed := false
	defer func() {
		if committed {
			return
		}
		for _, waitID := range registeredChildWaits {
			loop.engine.unregisterChildWait(waitID)
		}
	}()
	mailbox, err := restoreSignalMailbox(loop.mailbox.snapshot())
	if err != nil {
		return err
	}
	consumedChildWaits, err := mailbox.consumedChildWaitIDs(prepared.wire.Transition.ConsumedSignals())
	if err != nil {
		return err
	}
	if err := mailbox.commit(prepared.wire.Transition.ConsumedSignals()); err != nil {
		return err
	}
	for _, record := range prepared.wire.Effects {
		if record.Settlement == nil || record.Settlement.Status() == SettlementStatusUnknown {
			return errors.New("effect batch is not definitely settled")
		}
		waitID := WaitID{}
		if record.Effect.Target() == EffectTargetFramework {
			operation, err := frameworkEffectOperation(record.Effect.Payload())
			if err != nil {
				return errors.New("invalid prepared framework Effect")
			}
			switch operation {
			case frameworkEffectWait:
				key, _, err := decodeWaitRequest(record.Effect)
				if err != nil || record.WaitID == nil {
					return errors.New("invalid prepared wait Effect")
				}
				waitID = *record.WaitID
				if err := mailbox.registerWait(key, waitID, true); err != nil {
					return err
				}
			case frameworkEffectStartChild:
				if record.WaitID != nil {
					return errors.New("child-start Effect unexpectedly contains a WaitID")
				}
			case frameworkEffectWaitChildren:
				spec, err := decodeChildWaitEffect(record.Effect.Payload())
				if err != nil || record.WaitID == nil {
					return errors.New("invalid child-wait Effect")
				}
				waitID = *record.WaitID
				if err := mailbox.registerWait(spec.Key, waitID, false); err != nil {
					return err
				}
				immediate, err := loop.engine.registerChildWait(loop.controller.processID, waitID, spec)
				if err != nil {
					return err
				}
				registeredChildWaits = append(registeredChildWaits, waitID)
				if immediate != nil {
					immediateChildSignals = append(immediateChildSignals, *immediate)
				}
			default:
				return errors.New("unsupported prepared framework Effect")
			}
		}
		signal, err := newSignal(
			deriveSettlementSignalID(record.ID), waitID, time.Now(), record.Settlement.Payload(),
		)
		if err != nil {
			return err
		}
		if waitID.Valid() {
			if err := mailbox.enqueueWaitOpened(signal); err != nil {
				return err
			}
		} else {
			accepted, err := mailbox.enqueue(StatusRunning, signal)
			if err != nil || !accepted {
				return errors.Join(err, errors.New("internal settlement Signal was not accepted"))
			}
		}
	}
	for _, signal := range immediateChildSignals {
		if loop.usage.AcceptedSignals+uint64(len(prepared.wire.Effects))+1 > loop.limits.MaxSignals ||
			mailbox.pendingCount()+1 > loop.limits.MaxPendingSignals ||
			loop.usage.AcceptedSignals+uint64(len(prepared.wire.Effects))+1+
				loop.reservedBudget.Signals > loop.budget.Signals {
			waitID, _ := signal.WaitID()
			loop.engine.unregisterChildWait(waitID)
			return ErrResourceLimitExceeded
		}
		accepted, err := mailbox.enqueueChildCompletion(StatusRunning, signal)
		if err != nil || !accepted {
			waitID, _ := signal.WaitID()
			loop.engine.unregisterChildWait(waitID)
			return errors.Join(err, errors.New("immediate child completion Signal was not accepted"))
		}
	}
	loop.execution = prepared.candidate
	if loop.execution == nil {
		loop.execution, err = restoreExecution(loop.deployment.Definition(), prepared.wire.CandidateState)
		if err != nil {
			return err
		}
	}
	loop.lastStableState = prepared.wire.CandidateState
	loop.mailbox = mailbox
	loop.committedSteps = prepared.wire.StepSequence
	loop.usage.CommittedSteps = loop.committedSteps
	loop.usage.AcceptedSignals += uint64(len(prepared.wire.Effects))
	loop.usage.AcceptedSignals += uint64(len(immediateChildSignals))
	transition := prepared.wire.Transition
	loop.prepared = nil
	switch transition.Kind() {
	case TransitionKindContinue:
		loop.status = StatusRunning
	case TransitionKindWait:
		waitID, _ := transition.WaitID()
		shouldWait, err := loop.mailbox.enterWait(waitID)
		if err != nil {
			return err
		}
		if shouldWait {
			loop.status = StatusWaiting
			loop.currentWaitID = waitID
		} else {
			loop.status = StatusRunning
			loop.currentWaitID = WaitID{}
		}
	case TransitionKindPause:
		loop.status = StatusPaused
		loop.pauseReason, _ = transition.Reason()
	case TransitionKindComplete:
		loop.finalOutput, _ = transition.Output()
		loop.commitTermination(completedOutcome())
	case TransitionKindFail:
		failure, _ := transition.Failure()
		outcome, _ := failedOutcome(failure)
		loop.commitTermination(outcome)
	default:
		return ErrInvalidTransition
	}
	loop.updateView()
	payload, _ := json.Marshal(stepCommittedEventPayload{ProcessStatus: loop.status.String()})
	loop.publishEvent(ctx, EventStepCommitted, EventPhaseCommitted, loop.committedSteps, EffectID{}, payload)
	for _, waitID := range consumedChildWaits {
		loop.engine.unregisterChildWait(waitID)
	}
	committed = true
	return nil
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

func (loop *processLoop) fail(ctx context.Context, kind FailureKind, code string, err error) {
	failure, failureErr := failureFromError(kind, code, err)
	if failureErr != nil {
		failure, _ = NewFailure(FailureKindContract, "engine.failure.invalid", "Engine could not construct a valid failure")
	}
	outcome, _ := failedOutcome(failure)
	loop.commitTermination(outcome)
}

func (loop *processLoop) commitTermination(outcome stepOutcome) {
	termination, err := resolveTermination(terminationFacts{
		kill: loop.pendingControl.kill, deadline: loop.pendingControl.deadline,
		cancellation: loop.pendingControl.cancellation, outcome: outcome,
	})
	if err != nil {
		failure, _ := NewFailure(FailureKindContract, "engine.termination.invalid", err.Error())
		termination = terminationForFailure(failure)
	}
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

func (loop *processLoop) observeHostError(ctx context.Context) {
	if err := ctx.Err(); err != nil {
		loop.recordHostTermination(err)
	}
}
