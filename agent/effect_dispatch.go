package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"
)

func (loop *processLoop) dispatchPrepared(ctx context.Context, hostDone *<-chan struct{}) {
	for index := range loop.prepared.wire.Effects {
		record := &loop.prepared.wire.Effects[index]
		if record.Settlement != nil {
			continue
		}
		if loop.prepared.fromSnapshot && record.Effect.Target() == EffectTargetDispatcher {
			policy := dispatcherReplayPolicy(loop.deployment.effectDispatcher(), record.Effect)
			if policy != ReplayPolicySameIdentity {
				settlement, _ := NewSettlement(record.ID, SettlementStatusUnknown, json.RawMessage("null"))
				record.Settlement = &settlement
				continue
			}
		}
		if record.Effect.Target() == EffectTargetFramework {
			startedAt := loop.publishEffectStarted(
				ctx, loop.prepared.wire.StepSequence, record.ID, EffectTargetFramework,
			)
			loop.dispatchFrameworkEffect(ctx, record)
			loop.publishSettlementEvent(
				ctx, record.ID, EffectTargetFramework, record.Settlement.Status(), startedAt,
			)
			continue
		}
		loop.dispatchStrategyEffect(ctx, hostDone, uint32(index), record)
	}
}

func (loop *processLoop) dispatchFrameworkEffect(ctx context.Context, record *preparedEffectWire) {
	var header struct {
		Operation string `json:"operation"`
	}
	if err := json.Unmarshal(record.Effect.Payload(), &header); err != nil {
		loop.markFrameworkEffectUnknown(record)
		return
	}
	switch header.Operation {
	case frameworkEffectWait:
		_, payload, err := decodeWaitRequest(record.Effect)
		if err != nil {
			loop.markFrameworkEffectUnknown(record)
			return
		}
		waitID := deriveWaitID(record.ID)
		settlement, err := NewSettlement(record.ID, SettlementStatusSucceeded, payload)
		if err != nil {
			loop.markFrameworkEffectUnknown(record)
			return
		}
		record.WaitID = &waitID
		record.Settlement = &settlement
	case frameworkEffectStartChild:
		spec, err := decodeChildStartEffect(record.Effect.Payload())
		if err != nil {
			loop.markFrameworkEffectUnknown(record)
			return
		}
		result := loop.startChild(ctx, record.ID, spec)
		payload, err := encodeChildStartResult(result)
		if err != nil {
			loop.markFrameworkEffectUnknown(record)
			return
		}
		status := SettlementStatusSucceeded
		if _, failed := result.Failure(); failed {
			status = SettlementStatusFailed
		}
		settlement, err := NewSettlement(record.ID, status, payload)
		if err != nil {
			loop.markFrameworkEffectUnknown(record)
			return
		}
		record.Settlement = &settlement
	case frameworkEffectWaitChildren:
		spec, err := decodeChildWaitEffect(record.Effect.Payload())
		if err != nil {
			loop.markFrameworkEffectUnknown(record)
			return
		}
		payload, err := encodeChildWaitOpened(spec)
		if err != nil {
			loop.markFrameworkEffectUnknown(record)
			return
		}
		waitID := deriveWaitID(record.ID)
		settlement, err := NewSettlement(record.ID, SettlementStatusSucceeded, payload)
		if err != nil {
			loop.markFrameworkEffectUnknown(record)
			return
		}
		record.WaitID = &waitID
		record.Settlement = &settlement
	default:
		loop.markFrameworkEffectUnknown(record)
	}
}

func (loop *processLoop) markFrameworkEffectUnknown(record *preparedEffectWire) {
	settlement, _ := NewSettlement(record.ID, SettlementStatusUnknown, json.RawMessage("null"))
	record.Settlement = &settlement
}

type dispatchResult struct {
	settlement Settlement
	err        error
}

func (loop *processLoop) dispatchStrategyEffect(
	ctx context.Context,
	hostDone *<-chan struct{},
	index uint32,
	record *preparedEffectWire,
) {
	request := newEffectRequest(
		loop.controller.processID,
		loop.controller.deploymentRef,
		loop.controller.relation,
		loop.prepared.wire.StepSequence,
		index,
		record.ID,
		record.Effect,
	)
	startedAt := loop.publishEffectStarted(
		ctx, loop.prepared.wire.StepSequence, record.ID, EffectTargetDispatcher,
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
		delta, err := newDelta(loop.controller.processID, record.ID, sequence, time.Now(), payload)
		if err != nil || !loop.engine.observation.offerDelta(delta) {
			dropped.Add(1)
		}
	}
	result := make(chan dispatchResult, 1)
	dispatchCtx := context.WithoutCancel(ctx)
	go func() {
		settlement, err := dispatchEffect(loop.deployment.effectDispatcher(), dispatchCtx, request, emit)
		result <- dispatchResult{settlement: settlement, err: err}
	}()
	for {
		select {
		case command := <-loop.controller.commands:
			loop.applyCommand(ctx, command)
		case <-*hostDone:
			loop.recordHostTermination(ctx.Err())
			*hostDone = nil
		case outcome := <-result:
			acceptingDeltas.Store(false)
			settlement := outcome.settlement
			if outcome.err != nil || !settlement.Valid() || settlement.EffectID() != record.ID {
				settlement, _ = NewSettlement(record.ID, SettlementStatusUnknown, json.RawMessage("null"))
			}
			record.Settlement = &settlement
			if count := dropped.Load(); count > 0 {
				loop.usage.DroppedDeltas += count
				loop.updateView()
				payload, _ := json.Marshal(deltaDroppedEventPayload{DroppedDeltaCount: count})
				loop.publishEvent(ctx, EventDeltaDropped, EventPhaseAttempt, loop.prepared.wire.StepSequence, record.ID, payload)
			}
			loop.publishSettlementEvent(
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
