package agent

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
	command             processCommand
	deferred            []processCommand
	deferredHostErr     error
	preparedStateChange *preparedProcessStateChange
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

func (p *processLoop) run(ctx context.Context) {
	if p.restored {
		p.publishEvent(ctx, EventProcessRestored, EventPhaseCommitted, 0, EffectID{}, emptyEventPayload())
	} else {
		p.publishEvent(ctx, EventProcessStarted, EventPhaseCommitted, 0, EffectID{}, emptyEventPayload())
	}
	hostDone := ctx.Done()
	for !p.status.Terminal() {
		p.observeHostContext(ctx, &hostDone)
		switch {
		case p.quiescence != nil && p.prepared == nil:
			p.holdQuiescence(ctx, &hostDone)
		case p.prepared != nil:
			p.advancePrepared(ctx, &hostDone)
		default:
			if !p.applyPendingControl(ctx) {
				p.advanceStatus(ctx, &hostDone)
			}
		}
	}
	p.finish(ctx)
}

func (p *processLoop) advancePrepared(ctx context.Context, hostDone *<-chan struct{}) {
	if p.pendingControl.hasTerminalIntent() && p.prepared.hasUnknownSettlement() {
		p.discardPrepared()
		p.commitTermination(stepOutcome{})
		return
	}
	if !p.prepared.acknowledged && !p.acknowledgePrepared(ctx) {
		return
	}
	if p.prepared.hasUnknownSettlement() {
		p.waitForCommand(ctx, hostDone)
		return
	}
	if !p.prepared.allEffectsSettled() {
		p.dispatchPrepared(ctx, hostDone)
		return
	}
	if err := p.finalizePrepared(ctx); err != nil {
		p.discardPrepared()
		p.fail(FailureKindContract, "engine.finalize.invalid", err)
	}
}

func (p *processLoop) applyPendingControl(ctx context.Context) bool {
	if p.pendingControl.hasTerminalIntent() {
		p.commitTermination(stepOutcome{})
		return true
	}
	if p.pendingControl.pauseReason == "" || p.status != StatusRunning {
		return false
	}
	p.status = StatusPaused
	p.pauseReason = p.pendingControl.pauseReason
	p.pendingControl.pauseReason = ""
	p.updateView()
	p.publishEvent(ctx, EventProcessPaused, EventPhaseCommitted, 0, EffectID{}, emptyEventPayload())
	return true
}

func (p *processLoop) advanceStatus(ctx context.Context, hostDone *<-chan struct{}) {
	switch p.status {
	case StatusWaiting, StatusPaused:
		p.waitForCommand(ctx, hostDone)
	case StatusRunning:
		select {
		case command := <-p.controller.commands:
			p.applyCommand(ctx, command)
		default:
			p.prepareNextStep(ctx)
		}
	default:
		p.fail(FailureKindContract, "engine.status.invalid", fmt.Errorf("unexpected status %s", p.status))
	}
}

func (p *processLoop) waitForCommand(ctx context.Context, hostDone *<-chan struct{}) {
	select {
	case command := <-p.controller.commands:
		p.applyCommand(ctx, command)
	case <-*hostDone:
		p.recordHostTermination(ctx.Err())
		*hostDone = nil
	}
}

func (p *processLoop) observeHostContext(ctx context.Context, hostDone *<-chan struct{}) {
	if *hostDone == nil {
		return
	}
	select {
	case <-*hostDone:
		p.recordHostTermination(ctx.Err())
		*hostDone = nil
	default:
	}
}

func (p *processLoop) recordHostTermination(err error) {
	if errors.Is(err, context.DeadlineExceeded) {
		intent, _ := newDeadlineIntent(deadlineOwnerHost, "host context deadline reached")
		if !p.pendingControl.deadline.valid() {
			p.pendingControl.deadline = intent
		}
		return
	}
	intent, _ := newCancellationIntent(cancellationOwnerHost, "host context canceled")
	if !p.pendingControl.cancellation.valid() {
		p.pendingControl.cancellation = intent
	}
}

func (p *processLoop) applyCommand(ctx context.Context, command processCommand) {
	if p.status.Terminal() {
		command.reply(processResponse{err: ErrProcessFinished})
		return
	}
	switch command.kind {
	case commandDeliver:
		p.deliver(ctx, command)
	case commandDeliverBatch:
		p.deliverBatch(ctx, command)
	case commandPause:
		p.requestPause(command)
	case commandResume:
		p.resume(ctx, command)
	case commandCancel:
		p.requestCancellation(command.cancellationIntent)
	case commandKill:
		p.requestKill(command)
	case commandResolveEffect:
		p.resolveEffect(command)
	case commandQueryUnknownEffectIDs:
		command.reply(processResponse{unknownEffectIDs: p.unknownEffectIDs()})
	case commandCapture:
		snapshot, err := p.capture()
		command.reply(processResponse{snapshot: snapshot, err: err})
	case commandChildrenCompleted:
		delivered := p.deliverChildrenCompleted(ctx, command.internalSignal)
		command.reply(processResponse{accepted: delivered})
	case commandParentTerminated:
		p.recordParentTermination(command.parentTermination)
		command.reply(processResponse{accepted: true})
	case commandQuiesce:
		if command.release == nil || p.quiescence != nil {
			command.reply(processResponse{err: ErrProcessNotRunning})
			return
		}
		p.quiescence = &processQuiescence{command: command}
	default:
		command.reply(processResponse{err: ErrProcessNotRunning})
	}
}

func (p *processLoop) recordParentTermination(parent Termination) {
	if !parent.Valid() {
		return
	}
	if parent.Status() == StatusTimedOut {
		intent, _ := newDeadlineIntent(deadlineOwnerParent, "parent Process reached a deadline")
		if !p.pendingControl.deadline.valid() {
			p.pendingControl.deadline = intent
		}
		return
	}
	intent, _ := newCancellationIntent(
		cancellationOwnerParent,
		"parent Process reached terminal status "+parent.Status().String(),
	)
	if !p.pendingControl.cancellation.valid() {
		p.pendingControl.cancellation = intent
	}
}

func (p *processLoop) deliverChildrenCompleted(ctx context.Context, signal Signal) bool {
	if !resourceQuantitiesFit(p.limits.MaxSignals, p.usage.AcceptedSignals, 1) ||
		!resourceQuantitiesFit(p.limits.MaxPendingSignals, p.mailbox.pendingCount(), 1) ||
		!resourceQuantitiesFit(
			p.budget.Signals, p.usage.AcceptedSignals, p.reservedBudget.Signals, 1,
		) {
		p.fail(FailureKindExecution, "engine.limit.child_completion_signal", ErrResourceLimitExceeded)
		return false
	}
	accepted, err := p.mailbox.enqueueChildCompletion(p.status, signal)
	if err != nil {
		p.fail(FailureKindContract, "engine.child.completion.invalid", err)
		return false
	}
	if !accepted {
		return p.mailbox.contains(signal.ID())
	}
	p.usage.AcceptedSignals++
	if p.status == StatusWaiting {
		p.status = StatusRunning
		p.currentWaitID = WaitID{}
	}
	p.updateView()
	payload, _ := json.Marshal(signalAcceptedEventPayload{
		SignalID: signal.ID().String(), WaitID: commandSignalWaitID(signal),
	})
	p.publishEvent(ctx, EventSignalAccepted, EventPhaseCommitted, 0, EffectID{}, payload)
	return true
}

func (p *processLoop) holdQuiescence(ctx context.Context, hostDone *<-chan struct{}) {
	quiescence := p.quiescence
	quiescence.command.reply(processResponse{accepted: true})
	for {
		var applyGate <-chan struct{}
		if quiescence.preparedStateChange != nil {
			applyGate = quiescence.preparedStateChange.applyGate
		}
		select {
		case <-applyGate:
			quiescence.preparedStateChange.apply(ctx, p)
			close(quiescence.preparedStateChange.applied)
			quiescence.preparedStateChange = nil
		case <-quiescence.command.release:
			p.quiescence = nil
			if quiescence.deferredHostErr != nil && !p.status.Terminal() {
				p.recordHostTermination(quiescence.deferredHostErr)
			}
			for _, command := range quiescence.deferred {
				p.applyCommand(ctx, command)
				if p.status.Terminal() {
					return
				}
			}
			return
		case command := <-p.controller.commands:
			switch command.kind {
			case commandCapture, commandChildrenCompleted, commandParentTerminated:
				p.applyCommand(ctx, command)
			case commandStagePreparedProcessState:
				if quiescence.preparedStateChange != nil || command.preparedStateChange == nil {
					command.reply(processResponse{err: ErrInvalidPreparedWaitingSubtreeCancellation})
					continue
				}
				if err := command.preparedStateChange.validateSource(p); err != nil {
					command.reply(processResponse{err: err})
					continue
				}
				quiescence.preparedStateChange = command.preparedStateChange
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

func (p *processLoop) deliver(ctx context.Context, command processCommand) {
	if !command.signalRequest.Valid() {
		command.reply(processResponse{err: ErrInvalidSignalRequest})
		return
	}
	if p.mailbox.contains(command.signalRequest.ID()) {
		command.reply(processResponse{accepted: false})
		return
	}
	reserved := p.reservedSettlementSignals()
	if !resourceQuantitiesFit(p.limits.MaxSignals, p.usage.AcceptedSignals, reserved, 1) ||
		!resourceQuantitiesFit(p.limits.MaxPendingSignals, p.mailbox.pendingCount(), reserved, 1) ||
		!resourceQuantitiesFit(
			p.budget.Signals, p.usage.AcceptedSignals, p.reservedBudget.Signals, reserved, 1,
		) {
		command.reply(processResponse{err: ErrResourceLimitExceeded})
		return
	}
	signal, err := command.signalRequest.signal(time.Now())
	if err != nil {
		command.reply(processResponse{err: err})
		return
	}
	accepted, err := p.mailbox.enqueue(p.status, signal)
	if err != nil {
		command.reply(processResponse{err: err})
		return
	}
	if accepted && p.status == StatusWaiting {
		p.status = StatusRunning
		p.currentWaitID = WaitID{}
		p.updateView()
	}
	if accepted {
		p.usage.AcceptedSignals++
		p.updateView()
		payload, _ := json.Marshal(signalAcceptedEventPayload{
			SignalID: signal.ID().String(), WaitID: commandWaitID(command.signalRequest),
		})
		p.publishEvent(ctx, EventSignalAccepted, EventPhaseCommitted, 0, EffectID{}, payload)
	}
	command.reply(processResponse{accepted: accepted})
}

func (p *processLoop) deliverBatch(ctx context.Context, command processCommand) {
	if len(command.signalRequests) == 0 {
		command.reply(processResponse{err: ErrInvalidSignalRequest})
		return
	}
	count := uint64(len(command.signalRequests))
	reserved := p.reservedSettlementSignals()
	if !resourceQuantitiesFit(p.limits.MaxSignals, p.usage.AcceptedSignals, reserved, count) ||
		!resourceQuantitiesFit(p.limits.MaxPendingSignals, p.mailbox.pendingCount(), reserved, count) ||
		!resourceQuantitiesFit(p.budget.Signals, p.usage.AcceptedSignals, p.reservedBudget.Signals, reserved, count) {
		command.reply(processResponse{err: ErrResourceLimitExceeded})
		return
	}
	candidate := p.mailbox.clone()
	status := p.status
	signals := make([]Signal, 0, len(command.signalRequests))
	for _, request := range command.signalRequests {
		if !request.Valid() {
			command.reply(processResponse{err: ErrInvalidSignalRequest})
			return
		}
		if candidate.contains(request.ID()) {
			command.reply(processResponse{accepted: false})
			return
		}
		signal, err := request.signal(time.Now())
		if err != nil {
			command.reply(processResponse{err: err})
			return
		}
		accepted, err := candidate.enqueue(status, signal)
		if err != nil || !accepted {
			command.reply(processResponse{err: err, accepted: false})
			return
		}
		if status == StatusWaiting {
			if _, addressed := signal.WaitID(); addressed {
				status = StatusRunning
			}
		}
		signals = append(signals, signal)
	}
	p.mailbox = candidate
	if p.status == StatusWaiting && status == StatusRunning {
		p.status = StatusRunning
		p.currentWaitID = WaitID{}
	}
	p.usage.AcceptedSignals += count
	p.updateView()
	for index, signal := range signals {
		payload, _ := json.Marshal(signalAcceptedEventPayload{SignalID: signal.ID().String(), WaitID: commandWaitID(command.signalRequests[index])})
		p.publishEvent(ctx, EventSignalAccepted, EventPhaseCommitted, 0, EffectID{}, payload)
	}
	command.reply(processResponse{accepted: true})
}

func (p *processLoop) requestPause(command processCommand) {
	if err := validateTerminationReason(command.reason); err != nil {
		command.reply(processResponse{err: fmt.Errorf("%w: %w", ErrInvalidProcessControl, err)})
		return
	}
	if p.status != StatusRunning {
		command.reply(processResponse{err: ErrProcessNotRunning})
		return
	}
	if p.pendingControl.pauseReason == "" {
		p.pendingControl.pauseReason = command.reason
	}
	command.reply(processResponse{})
}

func (p *processLoop) resume(ctx context.Context, command processCommand) {
	if p.status != StatusPaused {
		command.reply(processResponse{err: ErrProcessNotRunning})
		return
	}
	p.status = StatusRunning
	p.pauseReason = ""
	p.pendingControl.pauseReason = ""
	p.updateView()
	p.publishEvent(ctx, EventProcessResumed, EventPhaseCommitted, 0, EffectID{}, emptyEventPayload())
	command.reply(processResponse{})
}

func (p *processLoop) requestCancellation(intent cancellationIntent) {
	if !p.pendingControl.cancellation.valid() {
		p.pendingControl.cancellation = intent
	}
}

func (p *processLoop) requestKill(command processCommand) {
	intent, err := newKillIntent(command.reason)
	if err != nil {
		command.reply(processResponse{err: fmt.Errorf("%w: %w", ErrInvalidProcessControl, err)})
		return
	}
	if !p.pendingControl.kill.valid() {
		p.pendingControl.kill = intent
	}
	command.reply(processResponse{})
}

func (p *processLoop) resolveEffect(command processCommand) {
	if p.prepared == nil || !command.settlement.Valid() || command.settlement.Status() == SettlementStatusUnknown {
		command.reply(processResponse{err: ErrEffectNotPending})
		return
	}
	for index := range p.prepared.wire.Effects {
		effect := &p.prepared.wire.Effects[index]
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

func (p *processLoop) finish(ctx context.Context) {
	eventPayload := processFinishedEventPayload{
		ProcessStatus: p.status, TerminationCause: p.termination.Cause(),
	}
	if failure, failed := p.termination.Failure(); failed {
		eventPayload.FailureKind = failure.Kind()
		eventPayload.FailureCode = failure.Code()
	}
	payload, _ := json.Marshal(eventPayload)
	p.publishEvent(ctx, EventProcessFinished, EventPhaseCommitted, 0, EffectID{}, payload)
	snapshot, err := p.capture()
	p.controller.complete(p.result(), snapshot, err)
	p.engine.processFinished(p.controller)
	p.controller.markTreeSettled()
}

func (p *processLoop) updateView() {
	p.controller.updateView(p.status, p.currentWaitID, p.usage)
}

func (p *processLoop) unknownEffectIDs() []EffectID {
	if p.prepared == nil {
		return nil
	}
	var ids []EffectID
	for _, effect := range p.prepared.wire.Effects {
		if effect.Settlement != nil && effect.Settlement.Status() == SettlementStatusUnknown {
			ids = append(ids, effect.ID)
		}
	}
	return ids
}

func (p *processLoop) reservedSettlementSignals() uint64 {
	if p.prepared == nil {
		return 0
	}
	return uint64(len(p.prepared.wire.Effects))
}

func (p pendingControl) hasTerminalIntent() bool {
	return p.kill.valid() || p.deadline.valid() || p.cancellation.valid()
}

func (p *preparedStep) hasUnknownSettlement() bool {
	for _, effect := range p.wire.Effects {
		if effect.Settlement != nil && effect.Settlement.Status() == SettlementStatusUnknown {
			return true
		}
	}
	return false
}

func (p *preparedStep) allEffectsSettled() bool {
	for _, effect := range p.wire.Effects {
		if effect.Settlement == nil || effect.Settlement.Status() == SettlementStatusUnknown {
			return false
		}
	}
	return true
}
