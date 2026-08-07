package agent2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type processLoop struct {
	engine     *Engine
	controller *processController
	deployment Deployment
	execution  Execution

	startedAt            time.Time
	finishedAt           time.Time
	status               Status
	committedSteps       uint64
	processEventSequence uint64
	lastStableState      ExecutionState
	mailbox              signalMailbox
	prepared             *preparedStep
	currentWaitID        WaitID
	pauseReason          string
	pendingControl       pendingControl
	finalOutput          Output
	termination          Termination
	limits               Limits
	treeLimits           TreeLimits
	budget               Budget
	reservedBudget       Budget
	capabilities         CapabilitySet
	usage                Usage
	restored             bool
	quiescence           *processQuiescence
}

type processQuiescence struct {
	command         processCommand
	deferred        []processCommand
	deferredHostErr error
	plannedState    *plannedProcessState
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

func newProcessLoop(
	engine *Engine,
	controller *processController,
	deployment Deployment,
	execution Execution,
	state ExecutionState,
	startedAt time.Time,
	limits Limits,
) *processLoop {
	return &processLoop{
		engine: engine, controller: controller, deployment: deployment, execution: execution,
		startedAt: startedAt, status: StatusRunning, lastStableState: state,
		mailbox: newSignalMailbox(), limits: limits, treeLimits: engine.treeLimits,
		budget: controller.budget, capabilities: controller.capabilities,
	}
}

func (loop *processLoop) run(ctx context.Context) {
	if loop.restored {
		loop.publishEvent(ctx, EventProcessRestored, EventPhaseCommitted, 0, EffectID{}, emptyEventPayload())
	} else {
		loop.publishEvent(ctx, EventProcessStarted, EventPhaseCommitted, 0, EffectID{}, emptyEventPayload())
	}
	hostDone := ctx.Done()
	for !loop.status.Terminal() {
		loop.observeHostContext(ctx, &hostDone)
		if loop.quiescence != nil && loop.prepared == nil {
			loop.holdQuiescence(ctx, &hostDone)
			continue
		}
		if loop.prepared != nil {
			if loop.pendingControl.hasTerminalIntent() && loop.prepared.hasUnknownSettlement() {
				loop.discardPrepared()
				loop.commitTermination(stepOutcome{})
				continue
			}
			if !loop.prepared.acknowledged {
				if !loop.acknowledgePrepared(ctx) {
					continue
				}
			}
			if loop.prepared.hasUnknownSettlement() {
				loop.waitForCommand(ctx, &hostDone)
				continue
			}
			if !loop.prepared.allEffectsSettled() {
				loop.dispatchPrepared(ctx, &hostDone)
				continue
			}
			if err := loop.finalizePrepared(ctx); err != nil {
				loop.discardPrepared()
				loop.fail(ctx, FailureKindContract, "engine.finalize.invalid", err)
			}
			continue
		}
		if loop.pendingControl.hasTerminalIntent() {
			loop.commitTermination(stepOutcome{})
			continue
		}
		if loop.pendingControl.pauseReason != "" && loop.status == StatusRunning {
			loop.status = StatusPaused
			loop.pauseReason = loop.pendingControl.pauseReason
			loop.pendingControl.pauseReason = ""
			loop.updateView()
			loop.publishEvent(ctx, EventProcessPaused, EventPhaseCommitted, 0, EffectID{}, emptyEventPayload())
			continue
		}
		switch loop.status {
		case StatusWaiting, StatusPaused:
			loop.waitForCommand(ctx, &hostDone)
		case StatusRunning:
			select {
			case command := <-loop.controller.commands:
				loop.applyCommand(ctx, command)
			default:
				loop.prepareNextStep(ctx)
			}
		default:
			loop.fail(ctx, FailureKindContract, "engine.status.invalid", fmt.Errorf("unexpected status %s", loop.status))
		}
	}
	loop.finish(ctx)
}

func (loop *processLoop) waitForCommand(ctx context.Context, hostDone *<-chan struct{}) {
	select {
	case command := <-loop.controller.commands:
		loop.applyCommand(ctx, command)
	case <-*hostDone:
		loop.recordHostTermination(ctx.Err())
		*hostDone = nil
	}
}

func (loop *processLoop) observeHostContext(ctx context.Context, hostDone *<-chan struct{}) {
	if *hostDone == nil {
		return
	}
	select {
	case <-*hostDone:
		loop.recordHostTermination(ctx.Err())
		*hostDone = nil
	default:
	}
}

func (loop *processLoop) recordHostTermination(err error) {
	if errors.Is(err, context.DeadlineExceeded) {
		intent, _ := newDeadlineIntent(deadlineOwnerHost, "host context deadline reached")
		if !loop.pendingControl.deadline.valid() {
			loop.pendingControl.deadline = intent
		}
		return
	}
	intent, _ := newCancellationIntent(cancellationOwnerHost, "host context canceled")
	if !loop.pendingControl.cancellation.valid() {
		loop.pendingControl.cancellation = intent
	}
}

func (loop *processLoop) applyCommand(ctx context.Context, command processCommand) {
	if loop.status.Terminal() {
		command.reply(processResponse{err: ErrProcessFinished})
		return
	}
	switch command.kind {
	case commandDeliver:
		loop.deliver(ctx, command)
	case commandPause:
		loop.requestPause(command)
	case commandResume:
		loop.resume(ctx, command)
	case commandCancel:
		loop.requestCancellation(command)
	case commandKill:
		loop.requestKill(command)
	case commandResolveEffect:
		loop.resolveEffect(command)
	case commandQueryUnknownEffectIDs:
		command.reply(processResponse{unknownEffectIDs: loop.unknownEffectIDs()})
	case commandCapture:
		snapshot, err := loop.capture()
		command.reply(processResponse{snapshot: snapshot, err: err})
	case commandChildrenCompleted:
		delivered := loop.deliverChildrenCompleted(ctx, command.internalSignal)
		command.reply(processResponse{accepted: delivered})
	case commandParentTerminated:
		loop.recordParentTermination(command.parentTermination)
		command.reply(processResponse{accepted: true})
	case commandQuiesce:
		if command.release == nil || loop.quiescence != nil {
			command.reply(processResponse{err: ErrProcessNotRunning})
			return
		}
		loop.quiescence = &processQuiescence{command: command}
	default:
		command.reply(processResponse{err: ErrProcessNotRunning})
	}
}

func (loop *processLoop) recordParentTermination(parent Termination) {
	if !parent.Valid() {
		return
	}
	if parent.Status() == StatusTimedOut {
		intent, _ := newDeadlineIntent(deadlineOwnerParent, "parent Process reached a deadline")
		if !loop.pendingControl.deadline.valid() {
			loop.pendingControl.deadline = intent
		}
		return
	}
	intent, _ := newCancellationIntent(
		cancellationOwnerParent,
		"parent Process reached terminal status "+parent.Status().String(),
	)
	if !loop.pendingControl.cancellation.valid() {
		loop.pendingControl.cancellation = intent
	}
}

func (loop *processLoop) deliverChildrenCompleted(ctx context.Context, signal Signal) bool {
	if loop.usage.AcceptedSignals >= loop.limits.MaxSignals ||
		loop.mailbox.pendingCount() >= loop.limits.MaxPendingSignals ||
		loop.usage.AcceptedSignals+1+loop.reservedBudget.Signals > loop.budget.Signals {
		loop.fail(ctx, FailureKindExecution, "engine.limit.child_completion_signal", ErrResourceLimitExceeded)
		return false
	}
	accepted, err := loop.mailbox.enqueueChildCompletion(loop.status, signal)
	if err != nil {
		loop.fail(ctx, FailureKindContract, "engine.child.completion.invalid", err)
		return false
	}
	if !accepted {
		return loop.mailbox.contains(signal.ID())
	}
	loop.usage.AcceptedSignals++
	if loop.status == StatusWaiting {
		loop.status = StatusRunning
		loop.currentWaitID = WaitID{}
	}
	loop.updateView()
	payload, _ := json.Marshal(signalAcceptedEventPayload{
		SignalID: signal.ID().String(), WaitID: commandSignalWaitID(signal),
	})
	loop.publishEvent(ctx, EventSignalAccepted, EventPhaseCommitted, 0, EffectID{}, payload)
	return true
}

func (loop *processLoop) holdQuiescence(ctx context.Context, hostDone *<-chan struct{}) {
	quiescence := loop.quiescence
	quiescence.command.reply(processResponse{accepted: true})
	for {
		var projectionApply <-chan struct{}
		if quiescence.plannedState != nil {
			projectionApply = quiescence.plannedState.applyGate
		}
		select {
		case <-projectionApply:
			quiescence.plannedState.apply(ctx, loop)
			close(quiescence.plannedState.applied)
			quiescence.plannedState = nil
		case <-quiescence.command.release:
			loop.quiescence = nil
			if quiescence.deferredHostErr != nil && !loop.status.Terminal() {
				loop.recordHostTermination(quiescence.deferredHostErr)
			}
			for _, command := range quiescence.deferred {
				loop.applyCommand(ctx, command)
				if loop.status.Terminal() {
					return
				}
			}
			return
		case command := <-loop.controller.commands:
			switch command.kind {
			case commandCapture, commandChildrenCompleted, commandParentTerminated:
				loop.applyCommand(ctx, command)
			case commandStagePlannedProcessState:
				if quiescence.plannedState != nil || command.plannedState == nil {
					command.reply(processResponse{err: ErrInvalidWaitingSubtreeCancellationPlan})
					continue
				}
				if err := command.plannedState.validateSource(loop); err != nil {
					command.reply(processResponse{err: err})
					continue
				}
				quiescence.plannedState = command.plannedState
				command.reply(processResponse{accepted: true})
			default:
				quiescence.deferred = append(quiescence.deferred, command)
			}
		case <-*hostDone:
			quiescence.deferredHostErr = ctx.Err()
			*hostDone = nil
		}
	}
}

func commandSignalWaitID(signal Signal) string {
	waitID, _ := signal.WaitID()
	return waitID.String()
}

func (loop *processLoop) deliver(ctx context.Context, command processCommand) {
	if !command.signalRequest.Valid() {
		command.reply(processResponse{err: ErrInvalidSignalRequest})
		return
	}
	if loop.mailbox.contains(command.signalRequest.ID()) {
		command.reply(processResponse{accepted: false})
		return
	}
	reserved := loop.reservedSettlementSignals()
	if loop.usage.AcceptedSignals+reserved >= loop.limits.MaxSignals ||
		loop.mailbox.pendingCount()+reserved >= loop.limits.MaxPendingSignals ||
		loop.usage.AcceptedSignals+reserved+1+loop.reservedBudget.Signals > loop.budget.Signals {
		command.reply(processResponse{err: ErrResourceLimitExceeded})
		return
	}
	signal, err := command.signalRequest.signal(time.Now())
	if err != nil {
		command.reply(processResponse{err: err})
		return
	}
	accepted, err := loop.mailbox.enqueue(loop.status, signal)
	if err != nil {
		command.reply(processResponse{err: err})
		return
	}
	if accepted && loop.status == StatusWaiting {
		loop.status = StatusRunning
		loop.currentWaitID = WaitID{}
		loop.updateView()
	}
	if accepted {
		loop.usage.AcceptedSignals++
		loop.updateView()
		payload, _ := json.Marshal(signalAcceptedEventPayload{
			SignalID: signal.ID().String(), WaitID: commandWaitID(command.signalRequest),
		})
		loop.publishEvent(ctx, EventSignalAccepted, EventPhaseCommitted, 0, EffectID{}, payload)
	}
	command.reply(processResponse{accepted: accepted})
}

func (loop *processLoop) requestPause(command processCommand) {
	if err := validateTerminationReason(command.reason); err != nil {
		command.reply(processResponse{err: fmt.Errorf("%w: %w", ErrInvalidProcessControl, err)})
		return
	}
	if loop.status != StatusRunning {
		command.reply(processResponse{err: ErrProcessNotRunning})
		return
	}
	if loop.pendingControl.pauseReason == "" {
		loop.pendingControl.pauseReason = command.reason
	}
	command.reply(processResponse{})
}

func (loop *processLoop) resume(ctx context.Context, command processCommand) {
	if loop.status != StatusPaused {
		command.reply(processResponse{err: ErrProcessNotRunning})
		return
	}
	loop.status = StatusRunning
	loop.pauseReason = ""
	loop.pendingControl.pauseReason = ""
	loop.updateView()
	loop.publishEvent(ctx, EventProcessResumed, EventPhaseCommitted, 0, EffectID{}, emptyEventPayload())
	command.reply(processResponse{})
}

func (loop *processLoop) requestCancellation(command processCommand) {
	intent, err := newCancellationIntent(cancellationOwnerHost, command.reason)
	if err != nil {
		command.reply(processResponse{err: fmt.Errorf("%w: %w", ErrInvalidProcessControl, err)})
		return
	}
	if !loop.pendingControl.cancellation.valid() {
		loop.pendingControl.cancellation = intent
	}
	command.reply(processResponse{})
}

func (loop *processLoop) requestKill(command processCommand) {
	intent, err := newKillIntent(command.reason)
	if err != nil {
		command.reply(processResponse{err: fmt.Errorf("%w: %w", ErrInvalidProcessControl, err)})
		return
	}
	if !loop.pendingControl.kill.valid() {
		loop.pendingControl.kill = intent
	}
	command.reply(processResponse{})
}

func (loop *processLoop) resolveEffect(command processCommand) {
	if loop.prepared == nil || !command.settlement.Valid() || command.settlement.Status() == SettlementStatusUnknown {
		command.reply(processResponse{err: ErrEffectNotPending})
		return
	}
	for index := range loop.prepared.wire.Effects {
		effect := &loop.prepared.wire.Effects[index]
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

func (loop *processLoop) finish(ctx context.Context) {
	payload, _ := json.Marshal(processFinishedEventPayload{
		ProcessStatus: loop.status.String(), TerminationCause: loop.termination.Cause().String(),
	})
	loop.publishEvent(ctx, EventProcessFinished, EventPhaseCommitted, 0, EffectID{}, payload)
	snapshot, err := loop.capture()
	loop.controller.complete(loop.result(), snapshot, err)
	loop.engine.processFinished(loop.controller)
	loop.controller.markTreeSettled()
}

func (loop *processLoop) updateView() {
	loop.controller.updateView(loop.status, loop.currentWaitID, loop.usage)
}

func (loop *processLoop) unknownEffectIDs() []EffectID {
	if loop.prepared == nil {
		return nil
	}
	var ids []EffectID
	for _, effect := range loop.prepared.wire.Effects {
		if effect.Settlement != nil && effect.Settlement.Status() == SettlementStatusUnknown {
			ids = append(ids, effect.ID)
		}
	}
	return ids
}

func (loop *processLoop) reservedSettlementSignals() uint64 {
	if loop.prepared == nil {
		return 0
	}
	return uint64(len(loop.prepared.wire.Effects))
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
