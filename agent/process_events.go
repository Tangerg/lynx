package agent

import (
	"context"
	"encoding/json"
	"math"
	"time"
)

func (p *processLoop) publishEvent(
	ctx context.Context,
	name string,
	phase EventPhase,
	step uint64,
	effectID EffectID,
	payload json.RawMessage,
) {
	if p.processEventSequence == math.MaxUint64 {
		return
	}
	nextSequence := p.processEventSequence + 1
	event, err := newEvent(eventSpec{
		processSequence: nextSequence,
		processID:       p.controller.processID,
		deploymentRef:   p.deployment.DeploymentRef(),
		relation:        p.controller.relation,
		stepSequence:    step,
		effectID:        effectID,
		name:            name,
		phase:           phase,
		occurredAt:      time.Now(),
		payload:         payload,
	})
	if err != nil {
		return
	}
	p.processEventSequence = nextSequence
	p.engine.observation.publishEvent(context.WithoutCancel(ctx), event)
}

func (p *processLoop) publishEffectStarted(
	ctx context.Context,
	step uint64,
	effectID EffectID,
	target EffectTarget,
) time.Time {
	payload, _ := json.Marshal(effectStartedEventPayload{EffectTarget: target})
	p.publishEvent(ctx, EventEffectStarted, EventPhaseAttempt, step, effectID, payload)
	return time.Now()
}

func (p *processLoop) publishSettlementEvent(
	ctx context.Context,
	effectID EffectID,
	target EffectTarget,
	status SettlementStatus,
	startedAt time.Time,
) {
	payload, err := json.Marshal(effectFinishedEventPayload{
		EffectTarget: target, SettlementStatus: status,
		DurationMS: time.Since(startedAt).Milliseconds(),
	})
	if err != nil {
		return
	}
	p.publishEvent(
		ctx, EventEffectFinished, EventPhaseAttempt,
		p.prepared.wire.StepSequence, effectID, payload,
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
