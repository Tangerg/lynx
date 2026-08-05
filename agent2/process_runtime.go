package agent2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

type processRuntime struct {
	engine     *Engine
	controller *processController
	deployment Deployment
	execution  Execution

	startedAt      time.Time
	finishedAt     time.Time
	status         Status
	committedSteps uint64
	eventSequence  uint64
	lastStable     ExecutionState
	mailbox        signalMailbox
	prepared       *preparedStep
	currentWaitID  WaitID
	pauseReason    string
	control        pendingControl
	output         Output
	termination    Termination
	limits         Limits
	usage          Usage
	restored       bool
}

type preparedStep struct {
	wire         preparedStepWire
	candidate    Execution
	acknowledged bool
	fromSnapshot bool
}

type pendingControl struct {
	kill         killIntent
	deadline     deadlineIntent
	cancellation cancellationIntent
	pauseReason  string
}

func newProcessRuntime(
	engine *Engine,
	controller *processController,
	deployment Deployment,
	execution Execution,
	state ExecutionState,
	startedAt time.Time,
	limits Limits,
) *processRuntime {
	return &processRuntime{
		engine: engine, controller: controller, deployment: deployment, execution: execution,
		startedAt: startedAt, status: StatusRunning, lastStable: state,
		mailbox: newSignalMailbox(), limits: limits,
	}
}

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
		pauseReason: wire.PauseReason, limits: wire.Limits, usage: wire.Usage,
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

func (runtime *processRuntime) run(ctx context.Context) {
	if runtime.restored {
		runtime.publishEvent(ctx, "agent.process.restored", EventPhaseCommitted, 0, EffectID{}, emptyEventPayload())
	} else {
		runtime.publishEvent(ctx, "agent.process.started", EventPhaseCommitted, 0, EffectID{}, emptyEventPayload())
	}
	hostDone := ctx.Done()
	for !runtime.status.Terminal() {
		runtime.observeHostContext(ctx, &hostDone)
		if runtime.prepared != nil {
			if runtime.control.hasTerminalIntent() && runtime.prepared.hasUnknownSettlement() {
				runtime.discardPrepared()
				runtime.commitTermination(stepOutcome{})
				continue
			}
			if !runtime.prepared.acknowledged {
				if !runtime.acknowledgePrepared(ctx) {
					continue
				}
			}
			if runtime.prepared.hasUnknownSettlement() {
				runtime.waitForCommand(ctx, &hostDone)
				continue
			}
			if !runtime.prepared.allEffectsSettled() {
				runtime.dispatchPrepared(ctx, &hostDone)
				continue
			}
			if err := runtime.finalizePrepared(ctx); err != nil {
				runtime.discardPrepared()
				runtime.fail(ctx, FailureKindContract, "engine.finalize.invalid", err)
			}
			continue
		}
		if runtime.control.hasTerminalIntent() {
			runtime.commitTermination(stepOutcome{})
			continue
		}
		if runtime.control.pauseReason != "" && runtime.status == StatusRunning {
			runtime.status = StatusPaused
			runtime.pauseReason = runtime.control.pauseReason
			runtime.control.pauseReason = ""
			runtime.updateView()
			runtime.publishEvent(ctx, "agent.process.paused", EventPhaseCommitted, 0, EffectID{}, emptyEventPayload())
			continue
		}
		switch runtime.status {
		case StatusWaiting, StatusPaused:
			runtime.waitForCommand(ctx, &hostDone)
		case StatusRunning:
			select {
			case command := <-runtime.controller.commands:
				runtime.applyCommand(ctx, command)
			default:
				runtime.prepareNextStep(ctx)
			}
		default:
			runtime.fail(ctx, FailureKindContract, "engine.status.invalid", fmt.Errorf("unexpected status %s", runtime.status))
		}
	}
	runtime.finish(ctx)
}

func (runtime *processRuntime) waitForCommand(ctx context.Context, hostDone *<-chan struct{}) {
	select {
	case command := <-runtime.controller.commands:
		runtime.applyCommand(ctx, command)
	case <-*hostDone:
		runtime.recordHostTermination(ctx.Err())
		*hostDone = nil
	}
}

func (runtime *processRuntime) observeHostContext(ctx context.Context, hostDone *<-chan struct{}) {
	if *hostDone == nil {
		return
	}
	select {
	case <-*hostDone:
		runtime.recordHostTermination(ctx.Err())
		*hostDone = nil
	default:
	}
}

func (runtime *processRuntime) recordHostTermination(err error) {
	if errors.Is(err, context.DeadlineExceeded) {
		intent, _ := newDeadlineIntent(deadlineOwnerHost, "host context deadline reached")
		if !runtime.control.deadline.valid() {
			runtime.control.deadline = intent
		}
		return
	}
	intent, _ := newCancellationIntent(cancellationOwnerHost, "host context cancelled")
	if !runtime.control.cancellation.valid() {
		runtime.control.cancellation = intent
	}
}

func (runtime *processRuntime) applyCommand(ctx context.Context, command processCommand) {
	if runtime.status.Terminal() {
		command.reply(processResponse{err: ErrProcessFinished})
		return
	}
	switch command.kind {
	case commandDeliver:
		runtime.deliver(ctx, command)
	case commandPause:
		runtime.requestPause(command)
	case commandResume:
		runtime.resume(ctx, command)
	case commandCancel:
		runtime.requestCancellation(command)
	case commandKill:
		runtime.requestKill(command)
	case commandResolveEffect:
		runtime.resolveEffect(command)
	case commandQueryUnknownEffects:
		command.reply(processResponse{effectIDs: runtime.unknownEffectIDs()})
	case commandCapture:
		snapshot, err := runtime.capture()
		command.reply(processResponse{snapshot: snapshot, err: err})
	default:
		command.reply(processResponse{err: ErrProcessNotRunning})
	}
}

func (runtime *processRuntime) deliver(ctx context.Context, command processCommand) {
	if !command.signal.Valid() {
		command.reply(processResponse{err: ErrInvalidSignalRequest})
		return
	}
	if runtime.mailbox.contains(command.signal.ID()) {
		command.reply(processResponse{accepted: false})
		return
	}
	reserved := runtime.reservedSettlementSignals()
	if runtime.usage.AcceptedSignals+reserved >= runtime.limits.MaxSignals ||
		runtime.mailbox.pendingCount()+reserved >= runtime.limits.MaxPendingSignals {
		command.reply(processResponse{err: ErrLimitExceeded})
		return
	}
	signal, err := command.signal.signal(time.Now())
	if err != nil {
		command.reply(processResponse{err: err})
		return
	}
	accepted, err := runtime.mailbox.enqueue(runtime.status, signal)
	if err != nil {
		command.reply(processResponse{err: err})
		return
	}
	if accepted && runtime.status == StatusWaiting {
		runtime.status = StatusRunning
		runtime.currentWaitID = WaitID{}
		runtime.updateView()
	}
	if accepted {
		runtime.usage.AcceptedSignals++
		runtime.updateView()
		payload, _ := json.Marshal(struct {
			SignalID string `json:"signal_id"`
			WaitID   string `json:"wait_id,omitempty"`
		}{SignalID: signal.ID().String(), WaitID: commandWaitID(command.signal)})
		runtime.publishEvent(ctx, "agent.signal.accepted", EventPhaseCommitted, 0, EffectID{}, payload)
	}
	command.reply(processResponse{accepted: accepted})
}

func (runtime *processRuntime) requestPause(command processCommand) {
	if err := validateTerminationReason(command.reason); err != nil {
		command.reply(processResponse{err: fmt.Errorf("%w: %v", ErrInvalidControl, err)})
		return
	}
	if runtime.status != StatusRunning {
		command.reply(processResponse{err: ErrProcessNotRunning})
		return
	}
	if runtime.control.pauseReason == "" {
		runtime.control.pauseReason = command.reason
	}
	command.reply(processResponse{})
}

func (runtime *processRuntime) resume(ctx context.Context, command processCommand) {
	if runtime.status != StatusPaused {
		command.reply(processResponse{err: ErrProcessNotRunning})
		return
	}
	runtime.status = StatusRunning
	runtime.pauseReason = ""
	runtime.control.pauseReason = ""
	runtime.updateView()
	runtime.publishEvent(ctx, "agent.process.resumed", EventPhaseCommitted, 0, EffectID{}, emptyEventPayload())
	command.reply(processResponse{})
}

func (runtime *processRuntime) requestCancellation(command processCommand) {
	intent, err := newCancellationIntent(cancellationOwnerHost, command.reason)
	if err != nil {
		command.reply(processResponse{err: fmt.Errorf("%w: %v", ErrInvalidControl, err)})
		return
	}
	if !runtime.control.cancellation.valid() {
		runtime.control.cancellation = intent
	}
	command.reply(processResponse{})
}

func (runtime *processRuntime) requestKill(command processCommand) {
	intent, err := newKillIntent(command.reason)
	if err != nil {
		command.reply(processResponse{err: fmt.Errorf("%w: %v", ErrInvalidControl, err)})
		return
	}
	if !runtime.control.kill.valid() {
		runtime.control.kill = intent
	}
	command.reply(processResponse{})
}

func (runtime *processRuntime) resolveEffect(command processCommand) {
	if runtime.prepared == nil || !command.settlement.Valid() || command.settlement.Status() == SettlementStatusUnknown {
		command.reply(processResponse{err: ErrEffectNotPending})
		return
	}
	for index := range runtime.prepared.wire.Effects {
		effect := &runtime.prepared.wire.Effects[index]
		if effect.ID != command.settlement.EffectID() {
			continue
		}
		if effect.Settlement == nil || effect.Settlement.Status() != SettlementStatusUnknown {
			command.reply(processResponse{err: ErrEffectNotPending})
			return
		}
		settlement := command.settlement
		effect.Settlement = &settlement
		command.reply(processResponse{})
		return
	}
	command.reply(processResponse{err: ErrEffectNotPending})
}

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

func (runtime *processRuntime) dispatchPrepared(ctx context.Context, hostDone *<-chan struct{}) {
	for index := range runtime.prepared.wire.Effects {
		record := &runtime.prepared.wire.Effects[index]
		if record.Settlement != nil {
			continue
		}
		if runtime.prepared.fromSnapshot && record.Effect.Target() == EffectTargetDispatcher {
			policy := dispatcherReplayPolicy(runtime.deployment.effectDispatcher(), record.Effect)
			if policy != ReplayPolicySameIdentity {
				settlement, _ := NewSettlement(record.ID, SettlementStatusUnknown, json.RawMessage("null"))
				record.Settlement = &settlement
				runtime.publishSettlementEvent(ctx, record.ID, SettlementStatusUnknown)
				continue
			}
		}
		if record.Effect.Target() == EffectTargetFramework {
			runtime.dispatchFrameworkEffect(record)
			continue
		}
		runtime.dispatchStrategyEffect(ctx, hostDone, uint32(index), record)
	}
}

func (runtime *processRuntime) dispatchFrameworkEffect(record *preparedEffectWire) {
	_, payload, err := decodeWaitRequest(record.Effect)
	if err != nil {
		settlement, _ := NewSettlement(record.ID, SettlementStatusUnknown, json.RawMessage("null"))
		record.Settlement = &settlement
		return
	}
	waitID := deriveWaitID(record.ID)
	settlement, err := NewSettlement(record.ID, SettlementStatusSucceeded, payload)
	if err != nil {
		settlement, _ = NewSettlement(record.ID, SettlementStatusUnknown, json.RawMessage("null"))
	}
	record.WaitID = &waitID
	record.Settlement = &settlement
}

type dispatchResult struct {
	settlement Settlement
	err        error
}

func (runtime *processRuntime) dispatchStrategyEffect(
	ctx context.Context,
	hostDone *<-chan struct{},
	index uint32,
	record *preparedEffectWire,
) {
	request := newEffectRequest(runtime.controller.id, runtime.prepared.wire.Sequence, index, record.ID, record.Effect)
	runtime.publishEvent(ctx, "agent.effect.started", EventPhaseAttempt, runtime.prepared.wire.Sequence, record.ID, emptyEventPayload())
	var deltaSequence atomic.Uint64
	var dropped atomic.Uint64
	var acceptingDeltas atomic.Uint32
	acceptingDeltas.Swap(1)
	emit := func(payload json.RawMessage) {
		if acceptingDeltas.Load() == 0 {
			return
		}
		sequence := deltaSequence.Add(1)
		delta, err := newDelta(runtime.controller.id, record.ID, sequence, time.Now(), payload)
		if err != nil || !runtime.engine.observation.offerDelta(delta) {
			dropped.Add(1)
		}
	}
	result := make(chan dispatchResult, 1)
	dispatchCtx := context.WithoutCancel(ctx)
	go func() {
		settlement, err := dispatchEffect(runtime.deployment.effectDispatcher(), dispatchCtx, request, emit)
		result <- dispatchResult{settlement: settlement, err: err}
	}()
	for {
		select {
		case command := <-runtime.controller.commands:
			runtime.applyCommand(ctx, command)
		case <-*hostDone:
			runtime.recordHostTermination(ctx.Err())
			*hostDone = nil
		case outcome := <-result:
			acceptingDeltas.Swap(0)
			settlement := outcome.settlement
			if outcome.err != nil || !settlement.Valid() || settlement.EffectID() != record.ID {
				settlement, _ = NewSettlement(record.ID, SettlementStatusUnknown, json.RawMessage("null"))
			}
			record.Settlement = &settlement
			if count := dropped.Load(); count > 0 {
				runtime.usage.DroppedDeltas += count
				runtime.updateView()
				payload, _ := json.Marshal(struct {
					Count uint64 `json:"count"`
				}{Count: count})
				runtime.publishEvent(ctx, "agent.delta.dropped", EventPhaseAttempt, runtime.prepared.wire.Sequence, record.ID, payload)
			}
			runtime.publishSettlementEvent(ctx, record.ID, settlement.Status())
			return
		}
	}
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
			return errors.New("Effect batch is not definitely settled")
		}
		waitID := WaitID{}
		if record.Effect.Target() == EffectTargetFramework {
			key, _, err := decodeWaitRequest(record.Effect)
			if err != nil || record.WaitID == nil {
				return errors.New("invalid prepared Framework Effect")
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

func (runtime *processRuntime) finish(ctx context.Context) {
	payload, _ := json.Marshal(struct {
		Status string `json:"status"`
		Cause  string `json:"cause"`
	}{Status: runtime.status.String(), Cause: runtime.termination.Cause().String()})
	runtime.publishEvent(ctx, "agent.process.finished", EventPhaseCommitted, 0, EffectID{}, payload)
	snapshot, err := runtime.capture()
	runtime.controller.complete(runtime.result(), snapshot, err)
}

func (runtime *processRuntime) result() Result {
	return Result{
		processID: runtime.controller.id, startedAt: runtime.startedAt,
		finishedAt: runtime.finishedAt, output: runtime.output,
		termination: runtime.termination, usage: runtime.usage,
	}
}

func (runtime *processRuntime) updateView() {
	runtime.controller.updateView(runtime.status, runtime.currentWaitID, runtime.usage)
}

func (runtime *processRuntime) capture() (Snapshot, error) {
	wire := processSnapshotWire{
		SchemaVersion: processSnapshotSchemaVersion, ProcessID: runtime.controller.id,
		Deployment: runtime.deployment.Reference(), StartedAt: runtime.startedAt,
		Status: runtime.status, CommittedSteps: runtime.committedSteps, EventSequence: runtime.eventSequence,
		Limits: runtime.limits, Usage: runtime.usage,
		LastStable: runtime.lastStable, Mailbox: runtime.mailbox.snapshot(),
		PauseReason: runtime.pauseReason, PendingControl: runtime.control.wire(),
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

func (runtime *processRuntime) publishEvent(
	ctx context.Context,
	name string,
	phase EventPhase,
	step uint64,
	effectID EffectID,
	payload json.RawMessage,
) {
	runtime.eventSequence++
	event, err := newEvent(runtime.eventSequence, runtime.controller.id, step, effectID, name, phase, time.Now(), payload)
	if err != nil {
		return
	}
	runtime.engine.observation.publishEvent(context.WithoutCancel(ctx), event)
}

func (runtime *processRuntime) publishSettlementEvent(ctx context.Context, effectID EffectID, status SettlementStatus) {
	payload, err := json.Marshal(struct {
		Status string `json:"status"`
	}{Status: status.String()})
	if err != nil {
		return
	}
	runtime.publishEvent(
		ctx, "agent.effect.finished", EventPhaseAttempt,
		runtime.prepared.wire.Sequence, effectID, payload,
	)
}

func (runtime *processRuntime) unknownEffectIDs() []EffectID {
	if runtime.prepared == nil {
		return nil
	}
	var ids []EffectID
	for _, effect := range runtime.prepared.wire.Effects {
		if effect.Settlement != nil && effect.Settlement.Status() == SettlementStatusUnknown {
			ids = append(ids, effect.ID)
		}
	}
	return ids
}

func (runtime *processRuntime) reservedSettlementSignals() uint64 {
	if runtime.prepared == nil {
		return 0
	}
	return uint64(len(runtime.prepared.wire.Effects))
}

func emptyEventPayload() json.RawMessage { return json.RawMessage("{}") }

func commandWaitID(request SignalRequest) string {
	waitID, addressed := request.WaitID()
	if !addressed {
		return ""
	}
	return waitID.String()
}

func (control pendingControl) hasTerminalIntent() bool {
	return control.kill.valid() || control.deadline.valid() || control.cancellation.valid()
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

func (prepared *preparedStep) hasUnknownSettlement() bool {
	for _, effect := range prepared.wire.Effects {
		if effect.Settlement != nil && effect.Settlement.Status() == SettlementStatusUnknown {
			return true
		}
	}
	return false
}

func (prepared *preparedStep) allEffectsSettled() bool {
	for _, effect := range prepared.wire.Effects {
		if effect.Settlement == nil || effect.Settlement.Status() == SettlementStatusUnknown {
			return false
		}
	}
	return true
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

func failureFromError(kind FailureKind, code string, err error) (Failure, error) {
	message := "unknown error"
	if err != nil {
		message = strings.TrimSpace(err.Error())
	}
	if message == "" {
		message = "unknown error"
	}
	if len(message) > maxFailureMessageBytes {
		message = message[:maxFailureMessageBytes]
	}
	return NewFailure(kind, code, message)
}

func failureKindForError(err error) FailureKind {
	var panicError executionPanic
	if errors.As(err, &panicError) {
		return FailureKindPanic
	}
	return FailureKindExecution
}

type executionPanic struct{ value any }

func (panicError executionPanic) Error() string {
	return fmt.Sprintf("Execution panicked: %v", panicError.value)
}

func startExecution(definition Definition, input Input) (execution Execution, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			execution = nil
			err = executionPanic{value: recovered}
		}
	}()
	execution, err = definition.Start(input)
	if err == nil && nilInterface(execution) {
		return nil, errors.New("Definition.Start returned nil execution")
	}
	return execution, err
}

func restoreExecution(definition Definition, state ExecutionState) (execution Execution, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			execution = nil
			err = executionPanic{value: recovered}
		}
	}()
	execution, err = definition.Restore(state)
	if err == nil && nilInterface(execution) {
		return nil, errors.New("Definition.Restore returned nil execution")
	}
	return execution, err
}

func stepExecution(ctx context.Context, execution Execution, signals []Signal) (transition Transition, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			transition = Transition{}
			err = executionPanic{value: recovered}
		}
	}()
	return execution.Step(ctx, signals)
}

func captureExecution(execution Execution) (state ExecutionState, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			state = ExecutionState{}
			err = executionPanic{value: recovered}
		}
	}()
	state, err = execution.Snapshot()
	if err == nil && !state.Valid() {
		return ExecutionState{}, ErrInvalidExecutionState
	}
	return state, err
}

func acknowledgePreparedStep(
	acknowledger PreparedStepAcknowledger,
	ctx context.Context,
	snapshot Snapshot,
) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("prepared-step acknowledger panicked: %v", recovered)
		}
	}()
	return acknowledger.AcknowledgePreparedStep(ctx, snapshot)
}

func dispatcherReplayPolicy(dispatcher Dispatcher, effect Effect) (policy ReplayPolicy) {
	defer func() {
		if recover() != nil {
			policy = ReplayPolicyNever
		}
	}()
	policy = dispatcher.ReplayPolicy(effect)
	if policy != ReplayPolicyNever && policy != ReplayPolicySameIdentity {
		return ReplayPolicyNever
	}
	return policy
}

func dispatchEffect(
	dispatcher Dispatcher,
	ctx context.Context,
	request EffectRequest,
	emit DeltaEmitter,
) (settlement Settlement, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			settlement = Settlement{}
			err = fmt.Errorf("dispatcher panicked: %v", recovered)
		}
	}()
	return dispatcher.Dispatch(ctx, request, emit)
}
