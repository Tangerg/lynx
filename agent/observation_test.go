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
	event, err := newEvent(eventSpec{
		processSequence: 7,
		processID:       processID,
		deploymentRef:   deployment.DeploymentRef(),
		relation:        relation,
		stepSequence:    2,
		effectID:        effectID,
		name:            EventEffectStarted,
		phase:           EventPhaseAttempt,
		occurredAt:      time.Unix(20, 0),
		payload:         json.RawMessage(`{"attempt":1}`),
	})
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

func TestDeltaDeliveryPreservesValuesWithoutRequestCancellation(t *testing.T) {
	type contextKey struct{}
	const wantValue = "run-correlation"
	var (
		gotValue string
		gotDone  <-chan struct{}
	)
	bus := newObservationBus(nil, []DeltaListener{
		DeltaListenerFunc(func(ctx context.Context, _ Delta) {
			gotValue, _ = ctx.Value(contextKey{}).(string)
			gotDone = ctx.Done()
		}),
	}, 1)
	t.Cleanup(bus.close)

	processID, _ := ParseProcessID("process:delta-context")
	effectID, _ := ParseEffectID("process:delta-context:step:1:effect:0")
	delta, err := newDelta(processID, effectID, 1, time.Unix(30, 0), json.RawMessage(`{"text":"partial"}`))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.WithValue(t.Context(), contextKey{}, wantValue))
	if !bus.offerDelta(ctx, delta) {
		t.Fatal("delta was not accepted")
	}
	cancel()
	if err := bus.flushDeltas(t.Context()); err != nil {
		t.Fatal(err)
	}
	if gotValue != wantValue {
		t.Fatalf("listener context value = %q, want %q", gotValue, wantValue)
	}
	if gotDone != nil {
		t.Fatal("listener inherited request cancellation")
	}
}

func TestObservationFailuresAreCountedWithoutAffectingDelivery(t *testing.T) {
	bus := newObservationBus(
		[]EventListener{
			EventListenerFunc(func(context.Context, Event) { panic("event observer failed") }),
			EventListenerFunc(func(context.Context, Event) {}),
		},
		[]DeltaListener{
			DeltaListenerFunc(func(context.Context, Delta) { panic("delta observer failed") }),
			DeltaListenerFunc(func(context.Context, Delta) {}),
		},
		1,
	)
	t.Cleanup(bus.close)

	bus.publishEvent(t.Context(), Event{})
	if !bus.offerDelta(t.Context(), Delta{}) {
		t.Fatal("delta was not accepted")
	}
	if err := bus.flushDeltas(t.Context()); err != nil {
		t.Fatal(err)
	}

	counts := bus.failureCounts()
	if counts.EventListenerPanics() != 1 || counts.DeltaListenerPanics() != 1 {
		t.Fatalf(
			"observation failures = event %d, delta %d, want 1 each",
			counts.EventListenerPanics(), counts.DeltaListenerPanics(),
		)
	}
	bus.eventListenerPanics.Store(math.MaxUint64)
	bus.publishEvent(t.Context(), Event{})
	if got := bus.failureCounts().EventListenerPanics(); got != math.MaxUint64 {
		t.Fatalf("saturated event listener panic count = %d", got)
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
