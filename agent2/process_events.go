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

func emptyEventPayload() json.RawMessage { return json.RawMessage("{}") }

func commandWaitID(request SignalRequest) string {
	waitID, addressed := request.WaitID()
	if !addressed {
		return ""
	}
	return waitID.String()
}
