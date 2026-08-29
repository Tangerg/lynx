package agent

import (
	"context"
	"encoding/json"
	"fmt"
)

const nullJSON = "null"

func (p *processState) dispatchFrameworkEffect(ctx context.Context, record *preparedEffectWire) {
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
		// Child start crosses admission and initialization boundaries. treeRuntime
		// intercepts it and commits its fenced job completion atomically.
		p.markFrameworkEffectUnknown(record)
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

func (p *processState) markFrameworkEffectUnknown(record *preparedEffectWire) {
	settlement, _ := NewSettlement(record.ID, SettlementStatusUnknown, json.RawMessage(nullJSON))
	record.Settlement = &settlement
}

func (p *processState) markFrameworkEffectUnknownByID(effectID EffectID) {
	if p.prepared == nil {
		return
	}
	for index := range p.prepared.wire.Effects {
		record := &p.prepared.wire.Effects[index]
		if record.ID == effectID && record.Settlement == nil {
			p.markFrameworkEffectUnknown(record)
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
