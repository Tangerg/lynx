package agent

import (
	"context"
	"encoding/json"
	"time"
)

func (loop *processLoop) publishEvent(
	ctx context.Context,
	name string,
	phase EventPhase,
	step uint64,
	effectID EffectID,
	payload json.RawMessage,
) {
	loop.processEventSequence++
	event, err := newEvent(
		loop.processEventSequence, loop.controller.processID, loop.deployment.DeploymentRef(),
		loop.controller.relation, step, effectID, name, phase, time.Now(), payload,
	)
	if err != nil {
		return
	}
	loop.engine.observation.publishEvent(context.WithoutCancel(ctx), event)
}

func (loop *processLoop) publishEffectStarted(
	ctx context.Context,
	step uint64,
	effectID EffectID,
	target EffectTarget,
) time.Time {
	payload, _ := json.Marshal(effectStartedEventPayload{EffectTarget: target.String()})
	loop.publishEvent(ctx, EventEffectStarted, EventPhaseAttempt, step, effectID, payload)
	return time.Now()
}

func (loop *processLoop) publishSettlementEvent(
	ctx context.Context,
	effectID EffectID,
	target EffectTarget,
	status SettlementStatus,
	startedAt time.Time,
) {
	payload, err := json.Marshal(effectFinishedEventPayload{
		EffectTarget: target.String(), SettlementStatus: status.String(),
		DurationMS: time.Since(startedAt).Milliseconds(),
	})
	if err != nil {
		return
	}
	loop.publishEvent(
		ctx, EventEffectFinished, EventPhaseAttempt,
		loop.prepared.wire.StepSequence, effectID, payload,
	)
}

func emptyEventPayload() json.RawMessage { return json.RawMessage("{}") }

func commandWaitID(request SignalRequest) string {
	waitID, addressed := request.WaitID()
	if !addressed {
		return ""
	}
	return waitID.String()
}
