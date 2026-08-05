package agent2

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

func (runtime *processRuntime) prepareNextStep(ctx context.Context) {
	if runtime.committedSteps >= runtime.limits.MaxSteps {
		runtime.fail(ctx, FailureKindExecution, "engine.limit.steps", ErrLimitExceeded)
		return
	}
	signals := runtime.mailbox.pending()
	transition, err := stepExecution(ctx, runtime.execution, signals)
	if err != nil {
		runtime.observeHostError(ctx)
		runtime.discardExecution()
		runtime.fail(ctx, failureKindForError(err), "execution.step.failed", err)
		return
	}
	if !transition.Valid() || uint64(transition.Consumed()) > uint64(len(signals)) {
		runtime.discardExecution()
		runtime.fail(ctx, FailureKindContract, "execution.transition.invalid", ErrInvalidTransition)
		return
	}
	effectCount := uint64(len(transition.Effects()))
	if runtime.usage.PreparedEffects+effectCount > runtime.limits.MaxEffects {
		runtime.discardExecution()
		runtime.fail(ctx, FailureKindExecution, "engine.limit.effects", ErrLimitExceeded)
		return
	}
	remainingPending := runtime.mailbox.pendingCount() - uint64(transition.Consumed())
	if runtime.usage.AcceptedSignals+effectCount > runtime.limits.MaxSignals ||
		remainingPending+effectCount > runtime.limits.MaxPendingSignals {
		runtime.discardExecution()
		runtime.fail(ctx, FailureKindExecution, "engine.limit.signals", ErrLimitExceeded)
		return
	}
	if output, completes := transition.Output(); completes {
		if err := runtime.deployment.Descriptor().ValidateOutput(output); err != nil {
			runtime.discardExecution()
			runtime.fail(ctx, FailureKindContract, "execution.output.invalid", err)
			return
		}
	}
	candidateState, err := captureExecution(runtime.execution)
	if err != nil {
		runtime.discardExecution()
		runtime.fail(ctx, failureKindForError(err), "execution.snapshot.failed", err)
		return
	}
	candidate, err := restoreExecution(runtime.deployment.Definition(), candidateState)
	if err != nil {
		runtime.discardExecution()
		runtime.fail(ctx, failureKindForError(err), "execution.snapshot.unrestorable", err)
		return
	}
	digest, err := executionStateDigest(runtime.lastStable)
	if err != nil {
		runtime.discardExecution()
		runtime.fail(ctx, FailureKindContract, "engine.last_stable.invalid", err)
		return
	}
	sequence := runtime.committedSteps + 1
	wire := preparedStepWire{
		Sequence: sequence, LastStableDigest: digest, CandidateState: candidateState,
		ConsumeThrough: runtime.mailbox.consumedSequence() + uint64(transition.Consumed()),
		Transition:     transition,
	}
	for index, effect := range transition.Effects() {
		wire.Effects = append(wire.Effects, preparedEffectWire{
			ID: deriveEffectID(runtime.controller.id, sequence, index), Effect: effect,
		})
	}
	runtime.prepared = &preparedStep{
		wire: wire, candidate: candidate, acknowledged: len(wire.Effects) == 0,
	}
	runtime.usage.PreparedEffects += effectCount
	runtime.updateView()
	runtime.publishEvent(ctx, "agent.step.prepared", EventPhaseAttempt, sequence, EffectID{}, emptyEventPayload())
}

func (runtime *processRuntime) acknowledgePrepared(ctx context.Context) bool {
	if runtime.engine.acknowledger == nil {
		runtime.prepared.acknowledged = true
		return true
	}
	snapshot, err := runtime.capture()
	if err == nil {
		err = acknowledgePreparedStep(runtime.engine.acknowledger, ctx, snapshot)
	}
	if err == nil {
		runtime.prepared.acknowledged = true
		return true
	}
	runtime.observeHostError(ctx)
	runtime.discardPrepared()
	runtime.fail(ctx, FailureKindExternal, "engine.prepared_acknowledgment.failed", err)
	return false
}

func (runtime *processRuntime) finalizePrepared(ctx context.Context) error {
	prepared := runtime.prepared
	mailbox, err := restoreSignalMailbox(runtime.mailbox.snapshot())
	if err != nil {
		return err
	}
	if err := mailbox.commit(prepared.wire.Transition.Consumed()); err != nil {
		return err
	}
	for _, record := range prepared.wire.Effects {
		if record.Settlement == nil || record.Settlement.Status() == SettlementStatusUnknown {
			return errors.New("effect batch is not definitely settled")
		}
		waitID := WaitID{}
		if record.Effect.Target() == EffectTargetFramework {
			key, _, err := decodeWaitRequest(record.Effect)
			if err != nil || record.WaitID == nil {
				return errors.New("invalid prepared framework Effect")
			}
			waitID = *record.WaitID
			if err := mailbox.registerWait(key, waitID); err != nil {
				return err
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
	runtime.execution = prepared.candidate
	if runtime.execution == nil {
		runtime.execution, err = restoreExecution(runtime.deployment.Definition(), prepared.wire.CandidateState)
		if err != nil {
			return err
		}
	}
	runtime.lastStable = prepared.wire.CandidateState
	runtime.mailbox = mailbox
	runtime.committedSteps = prepared.wire.Sequence
	runtime.usage.CommittedSteps = runtime.committedSteps
	runtime.usage.AcceptedSignals += uint64(len(prepared.wire.Effects))
	transition := prepared.wire.Transition
	runtime.prepared = nil
	switch transition.Kind() {
	case TransitionKindContinue:
		runtime.status = StatusRunning
	case TransitionKindWait:
		waitID, _ := transition.WaitID()
		shouldWait, err := runtime.mailbox.enterWait(waitID)
		if err != nil {
			return err
		}
		if shouldWait {
			runtime.status = StatusWaiting
			runtime.currentWaitID = waitID
		} else {
			runtime.status = StatusRunning
			runtime.currentWaitID = WaitID{}
		}
	case TransitionKindPause:
		runtime.status = StatusPaused
		runtime.pauseReason, _ = transition.Reason()
	case TransitionKindComplete:
		runtime.output, _ = transition.Output()
		runtime.commitTermination(completedOutcome())
	case TransitionKindFail:
		failure, _ := transition.Failure()
		outcome, _ := failedOutcome(failure)
		runtime.commitTermination(outcome)
	default:
		return ErrInvalidTransition
	}
	runtime.updateView()
	payload, _ := json.Marshal(struct {
		Status string `json:"status"`
	}{Status: runtime.status.String()})
	runtime.publishEvent(ctx, "agent.step.committed", EventPhaseCommitted, runtime.committedSteps, EffectID{}, payload)
	return nil
}

func (runtime *processRuntime) discardPrepared() {
	runtime.prepared = nil
	runtime.discardExecution()
}

func (runtime *processRuntime) discardExecution() {
	execution, err := restoreExecution(runtime.deployment.Definition(), runtime.lastStable)
	if err == nil {
		runtime.execution = execution
	} else {
		runtime.execution = nil
	}
}

func (runtime *processRuntime) fail(ctx context.Context, kind FailureKind, code string, err error) {
	failure, failureErr := failureFromError(kind, code, err)
	if failureErr != nil {
		failure, _ = NewFailure(FailureKindContract, "engine.failure.invalid", "Engine could not construct a valid failure")
	}
	outcome, _ := failedOutcome(failure)
	runtime.commitTermination(outcome)
}

func (runtime *processRuntime) commitTermination(outcome stepOutcome) {
	termination, err := resolveTermination(terminationFacts{
		kill: runtime.control.kill, deadline: runtime.control.deadline,
		cancellation: runtime.control.cancellation, outcome: outcome,
	})
	if err != nil {
		failure, _ := NewFailure(FailureKindContract, "engine.termination.invalid", err.Error())
		termination = terminationForFailure(failure)
	}
	runtime.termination = termination
	runtime.status = termination.Status()
	runtime.finishedAt = time.Now().Round(0).UTC()
	runtime.currentWaitID = WaitID{}
	runtime.pauseReason = ""
	runtime.control = pendingControl{}
	runtime.updateView()
}

func (runtime *processRuntime) observeHostError(ctx context.Context) {
	if err := ctx.Err(); err != nil {
		runtime.recordHostTermination(err)
	}
}
