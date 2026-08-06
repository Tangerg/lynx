package agent2

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestSignalKeepsDeliveryAndWaitIdentitySeparate(t *testing.T) {
	signalID, err := ParseSignalID("signal:42")
	if err != nil {
		t.Fatal(err)
	}
	waitID, err := ParseWaitID("wait:7")
	if err != nil {
		t.Fatal(err)
	}
	payload := json.RawMessage(` { "answer": true } `)
	signal, err := newSignal(signalID, waitID, time.Date(2026, time.August, 6, 8, 9, 10, 11, time.FixedZone("test", 8*60*60)), payload)
	if err != nil {
		t.Fatal(err)
	}
	payload[3] = 'x'
	if signal.ID() != signalID {
		t.Fatalf("Signal.ID() = %v, want %v", signal.ID(), signalID)
	}
	if got, ok := signal.WaitID(); !ok || got != waitID {
		t.Fatalf("Signal.WaitID() = %v, %t, want %v, true", got, ok, waitID)
	}
	if got := string(signal.Payload()); got != `{"answer":true}` {
		t.Fatalf("Signal.Payload() = %s", got)
	}
	if signal.ReceivedAt().Location() != time.UTC {
		t.Fatalf("Signal.ReceivedAt location = %v, want UTC", signal.ReceivedAt().Location())
	}
}

func TestSignalStrictJSONRoundTrip(t *testing.T) {
	signalID, _ := ParseSignalID("signal:1")
	signal, err := newSignal(signalID, WaitID{}, time.Unix(42, 0), json.RawMessage(`{"kind":"steer"}`))
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(signal)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Signal
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded.WaitID(); ok {
		t.Fatal("decoded Signal unexpectedly has a WaitID")
	}
	if decoded.ID() != signal.ID() || string(decoded.Payload()) != string(signal.Payload()) {
		t.Fatalf("decoded Signal = %+v, want %+v", decoded, signal)
	}
	if err := json.Unmarshal([]byte(`{"id":"signal:1","received_at":"2026-08-06T00:00:00Z","payload":{},"unknown":true}`), &decoded); !errors.Is(err, ErrInvalidSignal) {
		t.Fatalf("unknown field error = %v, want ErrInvalidSignal", err)
	}
}

func TestDispatcherEffectIsOpaqueAndImmutable(t *testing.T) {
	payload := json.RawMessage(` { "operation": "model" } `)
	effect, err := NewDispatcherEffect(payload)
	if err != nil {
		t.Fatal(err)
	}
	payload[3] = 'x'
	copyOfPayload := effect.Payload()
	copyOfPayload[0] = '['
	if effect.Target() != EffectTargetDispatcher || string(effect.Payload()) != `{"operation":"model"}` {
		t.Fatalf("Effect = target %s payload %s", effect.Target(), effect.Payload())
	}

	data, err := json.Marshal(effect)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Effect
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Target() != EffectTargetDispatcher || string(decoded.Payload()) != string(effect.Payload()) {
		t.Fatalf("decoded Effect = %+v, want %+v", decoded, effect)
	}
}

func TestSettlementPreservesUnknownWithoutImplyingRetry(t *testing.T) {
	effectID, _ := ParseEffectID("process:1:step:2:effect:0")
	settlement, err := NewSettlement(effectID, SettlementStatusUnknown, json.RawMessage(`{"reason":"connection_lost"}`))
	if err != nil {
		t.Fatal(err)
	}
	if settlement.Status() != SettlementStatusUnknown || settlement.EffectID() != effectID {
		t.Fatalf("Settlement = status %s effect %v", settlement.Status(), settlement.EffectID())
	}
	data, err := json.Marshal(settlement)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Settlement
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Status() != SettlementStatusUnknown || string(decoded.Payload()) != `{"reason":"connection_lost"}` {
		t.Fatalf("decoded Settlement = %+v", decoded)
	}
}

func TestWaitRequestKeepsEngineKeySeparateFromStrategySignalPayload(t *testing.T) {
	key, _ := ParseWaitKey("approval:tool:1")
	effect, err := RequestWait(key, json.RawMessage(`{"kind":"approval_opened","tool":"shell"}`))
	if err != nil {
		t.Fatal(err)
	}
	if effect.Target() != EffectTargetFramework {
		t.Fatalf("RequestWait target = %s, want framework", effect.Target())
	}
	decodedKey, signalPayload, err := decodeWaitRequest(effect)
	if err != nil {
		t.Fatal(err)
	}
	if decodedKey != key || string(signalPayload) != `{"kind":"approval_opened","tool":"shell"}` {
		t.Fatalf("decoded wait request = key %v payload %s", decodedKey, signalPayload)
	}

	data, err := json.Marshal(effect)
	if err != nil {
		t.Fatal(err)
	}
	var restored Effect
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if _, _, err := decodeWaitRequest(restored); err != nil {
		t.Fatal(err)
	}
}

func TestFrameworkAndChildProtocolsRejectPriorSchemaVersions(t *testing.T) {
	key, _ := ParseWaitKey("approval:prior-version")
	waitEffect, err := RequestWait(key, json.RawMessage(`{"kind":"opened"}`))
	if err != nil {
		t.Fatal(err)
	}
	priorWaitPayload := withSchemaVersion(t, waitEffect.Payload(), 1)
	if _, _, err := decodeWaitRequestPayload(priorWaitPayload); !errors.Is(err, ErrInvalidEffect) {
		t.Fatalf("prior Framework Effect error = %v, want ErrInvalidEffect", err)
	}

	childKey, _ := ParseChildKey("child:prior-version")
	processID, _ := ParseProcessID("process:prior-version")
	deployment := newChildTestDeployment(t)
	result := ChildStartResult{
		key: childKey, processID: processID, deploymentRef: deployment.DeploymentRef(),
	}
	currentResult, err := encodeChildStartResult(result)
	if err != nil {
		t.Fatal(err)
	}
	priorResult := withSchemaVersion(t, currentResult, 1)
	if _, err := decodeChildStartResult(priorResult); !errors.Is(err, ErrInvalidChildStart) {
		t.Fatalf("prior child protocol error = %v, want ErrInvalidChildStart", err)
	}
}

func withSchemaVersion(t *testing.T, payload json.RawMessage, version uint16) json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(version)
	if err != nil {
		t.Fatal(err)
	}
	fields["schema_version"] = encoded
	updated, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return updated
}

func TestFrameworkEffectRejectsUnknownOperations(t *testing.T) {
	data := []byte(`{"target":"framework","payload":{"operation":"tool","schema_version":1,"key":"approval","signal_payload":{}}}`)
	var effect Effect
	if err := json.Unmarshal(data, &effect); !errors.Is(err, ErrInvalidEffect) {
		t.Fatalf("unknown Framework Effect error = %v, want ErrInvalidEffect", err)
	}
}
