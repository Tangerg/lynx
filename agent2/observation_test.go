package agent2

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEventSeparatesAttemptFromCommittedFacts(t *testing.T) {
	processID, _ := ParseProcessID("process:1")
	effectID, _ := ParseEffectID("process:1:step:2:effect:0")
	deployment := newChildTestDeployment(t)
	relation := rootProcessRelation(processID)
	event, err := newEvent(
		7, processID, deployment.Reference(), relation, 2, effectID,
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
	if decoded.Phase() != EventPhaseAttempt || decoded.Name() != EventEffectStarted || decoded.Sequence() != 7 {
		t.Fatalf("decoded Event = %+v", decoded)
	}
	if decoded.DeploymentRef() != deployment.Reference() || decoded.Relation() != relation {
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
	if delta.Sequence() != 1 || string(delta.Payload()) != `{"text":"partial"}` {
		t.Fatalf("Delta = sequence %d payload %s", delta.Sequence(), delta.Payload())
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
