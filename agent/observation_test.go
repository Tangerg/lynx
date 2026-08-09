package agent

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"
)

func TestEventSeparatesAttemptFromCommittedFacts(t *testing.T) {
	processID, _ := ParseProcessID("process:1")
	effectID, _ := ParseEffectID("process:1:step:2:effect:0")
	deployment := newChildTestDeployment(t)
	relation := rootProcessRelation(processID)
	event, err := newEvent(
		7, processID, deployment.DeploymentRef(), relation, 2, effectID,
		EventEffectStarted, EventPhaseAttempt, time.Unix(20, 0),
		json.RawMessage(`{"attempt":1}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Event
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Phase() != EventPhaseAttempt || decoded.Name() != EventEffectStarted || decoded.ProcessSequence() != 7 {
		t.Fatalf("decoded Event = %+v", decoded)
	}
	if decoded.DeploymentRef() != deployment.DeploymentRef() || decoded.Relation() != relation {
		t.Fatalf("decoded Deployment = %s, relation = %#v", decoded.DeploymentRef(), decoded.Relation())
	}
	if got, ok := decoded.EffectID(); !ok || got != effectID {
		t.Fatalf("decoded EffectID = %v, %t", got, ok)
	}
}

func TestDeltaIsEffectLocalAndImmutable(t *testing.T) {
	processID, _ := ParseProcessID("process:1")
	effectID, _ := ParseEffectID("process:1:step:2:effect:0")
	payload := json.RawMessage(` { "text": "partial" } `)
	delta, err := newDelta(processID, effectID, 1, time.Unix(30, 0), payload)
	if err != nil {
		t.Fatal(err)
	}
	payload[3] = 'x'
	copyOfPayload := delta.Payload()
	copyOfPayload[0] = '['
	if delta.EffectSequence() != 1 || string(delta.Payload()) != `{"text":"partial"}` {
		t.Fatalf("Delta = sequence %d payload %s", delta.EffectSequence(), delta.Payload())
	}
	data, err := json.Marshal(delta)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Delta
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ProcessID() != processID || decoded.EffectID() != effectID {
		t.Fatalf("decoded Delta = %+v", decoded)
	}
}

func TestProcessEventSequenceAdvancesOnlyForConstructedEvents(t *testing.T) {
	deployment := newChildTestDeployment(t)
	processID, _ := ParseProcessID("process:event-sequence")
	relation := rootProcessRelation(processID)
	var events []Event
	engine, err := NewEngine(EngineConfig{EventListeners: []EventListener{
		EventListenerFunc(func(_ context.Context, event Event) { events = append(events, event) }),
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	loop := &processLoop{
		engine: engine,
		controller: &processController{
			processID: processID, relation: relation, deploymentRef: deployment.DeploymentRef(),
		},
		deployment: deployment,
	}

	loop.processEventSequence = 7
	loop.publishEvent(
		context.Background(), "invalid event name", EventPhaseAttempt,
		0, EffectID{}, emptyEventPayload(),
	)
	if loop.processEventSequence != 7 || len(events) != 0 {
		t.Fatalf("invalid Event changed sequence to %d or published %d facts", loop.processEventSequence, len(events))
	}

	loop.publishEvent(
		context.Background(), EventProcessStarted, EventPhaseCommitted,
		0, EffectID{}, emptyEventPayload(),
	)
	if loop.processEventSequence != 8 || len(events) != 1 || events[0].ProcessSequence() != 8 {
		t.Fatalf("valid Event sequence = %d, events = %#v", loop.processEventSequence, events)
	}

	loop.processEventSequence = math.MaxUint64
	loop.publishEvent(
		context.Background(), EventProcessResumed, EventPhaseCommitted,
		0, EffectID{}, emptyEventPayload(),
	)
	if loop.processEventSequence != math.MaxUint64 || len(events) != 1 {
		t.Fatalf("exhausted Event sequence wrapped to %d or published %d facts", loop.processEventSequence, len(events))
	}
}
