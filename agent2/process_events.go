package agent2

import (
	"context"
	"encoding/json"
	"time"
)

func (runtime *processRuntime) publishEvent(
	ctx context.Context,
	name string,
	phase EventPhase,
	step uint64,
	effectID EffectID,
	payload json.RawMessage,
) {
	runtime.eventSequence++
	event, err := newEvent(
		runtime.eventSequence, runtime.controller.id, runtime.deployment.Reference(),
		runtime.controller.relation, step, effectID, name, phase, time.Now(), payload,
	)
	if err != nil {
		return
	}
	runtime.engine.observation.publishEvent(context.WithoutCancel(ctx), event)
}

func (runtime *processRuntime) publishEffectStarted(
	ctx context.Context,
	step uint64,
	effectID EffectID,
	target EffectTarget,
) time.Time {
	payload, _ := json.Marshal(struct {
		Target string `json:"target"`
	}{Target: target.String()})
	runtime.publishEvent(ctx, EventEffectStarted, EventPhaseAttempt, step, effectID, payload)
	return time.Now()
}

func (runtime *processRuntime) publishSettlementEvent(
	ctx context.Context,
	effectID EffectID,
	target EffectTarget,
	status SettlementStatus,
	startedAt time.Time,
) {
	payload, err := json.Marshal(struct {
		Target     string `json:"target"`
		Status     string `json:"status"`
		DurationMS int64  `json:"duration_ms"`
	}{
		Target: target.String(), Status: status.String(),
		DurationMS: time.Since(startedAt).Milliseconds(),
	})
	if err != nil {
		return
	}
	runtime.publishEvent(
		ctx, EventEffectFinished, EventPhaseAttempt,
		runtime.prepared.wire.Sequence, effectID, payload,
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
