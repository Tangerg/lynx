package agent

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestExecutionStateOwnsOpaquePayload(t *testing.T) {
	payload := json.RawMessage(` { "round": 2 } `)
	state, err := NewExecutionState("interaction", 1, payload)
	if err != nil {
		t.Fatal(err)
	}
	payload[3] = 'x'
	copyOfPayload := state.Payload()
	copyOfPayload[0] = '['
	if got := string(state.Payload()); got != `{"round":2}` {
		t.Fatalf("ExecutionState.Payload() = %s", got)
	}
}

func TestExecutionStateStrictJSONRoundTrip(t *testing.T) {
	state, err := NewExecutionState("planning.goap", 3, json.RawMessage(`{"phase":"observe"}`))
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var decoded ExecutionState
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Kind() != state.Kind() || decoded.SchemaVersion() != state.SchemaVersion() || string(decoded.Payload()) != string(state.Payload()) {
		t.Fatalf("decoded state = %+v, want %+v", decoded, state)
	}
	if err := json.Unmarshal([]byte(`{"kind":"planning","schema_version":1,"payload":{},"unknown":true}`), &decoded); !errors.Is(err, ErrInvalidExecutionState) {
		t.Fatalf("unknown field error = %v, want ErrInvalidExecutionState", err)
	}
}

func TestExecutionStateRejectsInvalidEnvelope(t *testing.T) {
	for _, test := range []struct {
		kind    string
		version uint16
		payload json.RawMessage
	}{
		{kind: "", version: 1, payload: json.RawMessage(`{}`)},
		{kind: "interaction", version: 0, payload: json.RawMessage(`{}`)},
		{kind: "interaction", version: 1},
	} {
		if _, err := NewExecutionState(test.kind, test.version, test.payload); !errors.Is(err, ErrInvalidExecutionState) {
			t.Fatalf("NewExecutionState(%q, %d) error = %v, want ErrInvalidExecutionState", test.kind, test.version, err)
		}
	}
}

func FuzzExecutionStateJSONRoundTrip(f *testing.F) {
	f.Add([]byte(`{"kind":"interaction","schema_version":1,"payload":{"round":2}}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		var state ExecutionState
		if err := json.Unmarshal(data, &state); err != nil {
			return
		}
		encoded, err := json.Marshal(state)
		if err != nil {
			t.Fatal(err)
		}
		var decoded ExecutionState
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.Kind() != state.Kind() || decoded.SchemaVersion() != state.SchemaVersion() || string(decoded.Payload()) != string(state.Payload()) {
			t.Fatalf("round trip = %+v, want %+v", decoded, state)
		}
	})
}
