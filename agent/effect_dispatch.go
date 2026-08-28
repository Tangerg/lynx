package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"
)

const nullJSON = "null"

func (p *processLoop) dispatchPrepared(ctx context.Context, hostDone *<-chan struct{}) {
	for index := range p.prepared.wire.Effects {
		record := &p.prepared.wire.Effects[index]
		if record.Settlement != nil {
			continue
		}
		if p.prepared.fromSnapshot && record.Effect.Target() == EffectTargetDispatcher {
			policy := dispatcherReplayPolicy(p.deployment.effectDispatcher(), record.Effect)
			if policy != ReplayPolicySameIdentity {
				settlement, _ := NewSettlement(record.ID, SettlementStatusUnknown, json.RawMessage(nullJSON))
				record.Settlement = &settlement
				continue
			}
		}
		if record.Effect.Target() == EffectTargetFramework {
			startedAt := p.publishEffectStarted(
				ctx, p.prepared.wire.StepSequence, record.ID, EffectTargetFramework,
			)
			p.dispatchFrameworkEffect(ctx, record)
			p.publishSettlementEvent(
				ctx, record.ID, EffectTargetFramework, record.Settlement.Status(), startedAt,
			)
			continue
		}
		p.dispatchStrategyEffect(ctx, hostDone, uint32(index), record)
	}
}

func (p *processLoop) dispatchFrameworkEffect(ctx context.Context, record *preparedEffectWire) {
	var header struct {
		Operation frameworkEffectOperation `json:"operation"`
	}
	if err := json.Unmarshal(record.Effect.Payload(), &header); err != nil {
		p.markFrameworkEffectUnknown(record)
		return
	}
	switch header.Operation {
	case frameworkEffectWait:
		_, payload, err := decodeWaitRequest(record.Effect)
		if err != nil {
			p.markFrameworkEffectUnknown(record)
			return
		}
		waitID := deriveWaitID(record.ID)
		settlement, err := NewSettlement(record.ID, SettlementStatusSucceeded, payload)
		if err != nil {
			p.markFrameworkEffectUnknown(record)
			return
		}
		record.WaitID = &waitID
		record.Settlement = &settlement
	case frameworkEffectStartChild:
		spec, err := decodeChildStartEffect(record.Effect.Payload())
		if err != nil {
			p.markFrameworkEffectUnknown(record)
			return
		}
		result := p.startChild(ctx, record.ID, spec)
		payload, err := encodeChildStartResult(result)
		if err != nil {
			p.markFrameworkEffectUnknown(record)
			return
		}
		status := SettlementStatusSucceeded
		if _, failed := result.Failure(); failed {
			status = SettlementStatusFailed
		}
		settlement, err := NewSettlement(record.ID, status, payload)
		if err != nil {
			p.markFrameworkEffectUnknown(record)
			return
		}
		record.Settlement = &settlement
	case frameworkEffectWaitChildren:
		spec, err := decodeChildWaitEffect(record.Effect.Payload())
		if err != nil {
			p.markFrameworkEffectUnknown(record)
			return
		}
		payload, err := encodeChildWaitOpened(spec)
		if err != nil {
			p.markFrameworkEffectUnknown(record)
			return
		}
		waitID := deriveWaitID(record.ID)
		settlement, err := NewSettlement(record.ID, SettlementStatusSucceeded, payload)
		if err != nil {
			p.markFrameworkEffectUnknown(record)
			return
		}
		record.WaitID = &waitID
		record.Settlement = &settlement
	default:
		p.markFrameworkEffectUnknown(record)
	}
}

func (p *processLoop) markFrameworkEffectUnknown(record *preparedEffectWire) {
	settlement, _ := NewSettlement(record.ID, SettlementStatusUnknown, json.RawMessage(nullJSON))
	record.Settlement = &settlement
}

type dispatchResult struct {
	settlement Settlement
	err        error
}

func (p *processLoop) dispatchStrategyEffect(
	ctx context.Context,
	hostDone *<-chan struct{},
	index uint32,
	record *preparedEffectWire,
) {
	request := newEffectRequest(
		p.controller.processID,
		p.controller.deploymentRef,
		p.controller.relation,
		p.prepared.wire.StepSequence,
		index,
		record.ID,
		record.Effect,
	)
	startedAt := p.publishEffectStarted(
		ctx, p.prepared.wire.StepSequence, record.ID, EffectTargetDispatcher,
	)
	var deltaSequence atomic.Uint64
	var dropped atomic.Uint64
	var acceptingDeltas atomic.Bool
	acceptingDeltas.Store(true)
	emit := func(payload json.RawMessage) {
		if !acceptingDeltas.Load() {
			return
		}
		sequence := deltaSequence.Add(1)
		delta, err := newDelta(p.controller.processID, record.ID, sequence, time.Now(), payload)
		if err != nil || !p.engine.observation.offerDelta(ctx, delta) {
			dropped.Add(1)
		}
	}
	result := make(chan dispatchResult, 1)
	dispatchCtx := context.WithoutCancel(ctx)
	go func() {
		settlement, err := dispatchEffect(dispatchCtx, p.deployment.effectDispatcher(), request, emit)
		result <- dispatchResult{settlement: settlement, err: err}
	}()
	for {
		select {
		case command := <-p.controller.commands:
			p.applyCommand(ctx, command)
		case <-*hostDone:
			p.recordHostTermination(ctx.Err())
			*hostDone = nil
		case outcome := <-result:
			acceptingDeltas.Store(false)
			settlement := outcome.settlement
			if outcome.err != nil || !settlement.Valid() || settlement.EffectID() != record.ID {
				settlement, _ = NewSettlement(record.ID, SettlementStatusUnknown, json.RawMessage(nullJSON))
			}
			record.Settlement = &settlement
			if count := dropped.Load(); count > 0 {
				p.usage.DroppedDeltas = saturatingCountAdd(p.usage.DroppedDeltas, count)
				p.updateView()
				payload, _ := json.Marshal(deltaDroppedEventPayload{DroppedDeltaCount: count})
				p.publishEvent(ctx, EventDeltaDropped, EventPhaseAttempt, p.prepared.wire.StepSequence, record.ID, payload)
			}
			p.publishSettlementEvent(
				ctx, record.ID, EffectTargetDispatcher, settlement.Status(), startedAt,
			)
			return
		}
	}
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
	ctx context.Context,
	dispatcher Dispatcher,
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
