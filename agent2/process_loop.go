package agent2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	case commandQueryUnknownEffectIDs:
		command.reply(processResponse{effectIDs: runtime.unknownEffectIDs()})
	case commandCapture:
		snapshot, err := runtime.capture()
		command.reply(processResponse{snapshot: snapshot, err: err})
	case commandChildrenCompleted:
		runtime.deliverChildrenCompleted(ctx, command.internalSignal)
	default:
		command.reply(processResponse{err: ErrProcessNotRunning})
	}
}

func (runtime *processRuntime) deliverChildrenCompleted(ctx context.Context, signal Signal) {
	if runtime.usage.AcceptedSignals >= runtime.limits.MaxSignals ||
		runtime.mailbox.pendingCount() >= runtime.limits.MaxPendingSignals {
		runtime.fail(ctx, FailureKindExecution, "engine.limit.child_completion_signal", ErrLimitExceeded)
		return
	}
	accepted, err := runtime.mailbox.enqueueChildCompletion(runtime.status, signal)
	if err != nil {
		runtime.fail(ctx, FailureKindContract, "engine.child.completion.invalid", err)
		return
	}
	if !accepted {
		return
	}
	runtime.usage.AcceptedSignals++
	if runtime.status == StatusWaiting {
		runtime.status = StatusRunning
		runtime.currentWaitID = WaitID{}
	}
	runtime.updateView()
	payload, _ := json.Marshal(struct {
		SignalID string `json:"signal_id"`
		WaitID   string `json:"wait_id"`
	}{SignalID: signal.ID().String(), WaitID: commandSignalWaitID(signal)})
	runtime.publishEvent(ctx, "agent.signal.accepted", EventPhaseCommitted, 0, EffectID{}, payload)
}

func commandSignalWaitID(signal Signal) string {
	waitID, _ := signal.WaitID()
	return waitID.String()
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

func (runtime *processRuntime) finish(ctx context.Context) {
	payload, _ := json.Marshal(struct {
		Status string `json:"status"`
		Cause  string `json:"cause"`
	}{Status: runtime.status.String(), Cause: runtime.termination.Cause().String()})
	runtime.publishEvent(ctx, "agent.process.finished", EventPhaseCommitted, 0, EffectID{}, payload)
	snapshot, err := runtime.capture()
	runtime.controller.complete(runtime.result(), snapshot, err)
	runtime.engine.processFinished(runtime.controller)
}

func (runtime *processRuntime) updateView() {
	runtime.controller.updateView(runtime.status, runtime.currentWaitID, runtime.usage)
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

func (control pendingControl) hasTerminalIntent() bool {
	return control.kill.valid() || control.deadline.valid() || control.cancellation.valid()
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
