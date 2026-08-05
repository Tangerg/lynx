package agent2

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"
)

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
	var acceptingDeltas atomic.Bool
	acceptingDeltas.Store(true)
	emit := func(payload json.RawMessage) {
		if !acceptingDeltas.Load() {
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
			acceptingDeltas.Store(false)
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
