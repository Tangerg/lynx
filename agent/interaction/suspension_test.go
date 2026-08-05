package interaction_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/interaction"
)

func TestSuspensionJSONRoundTripAndResponseValidation(t *testing.T) {
	suspension := interaction.Suspension{
		SchemaVersion:  interaction.SuspensionSchemaVersion,
		ID:             "approval-1",
		Prompt:         json.RawMessage(`{"message":"approve?"}`),
		ResponseSchema: json.RawMessage(`{"type":"object","properties":{"approved":{"type":"boolean"}},"required":["approved"]}`),
		FrameworkState: json.RawMessage(`{"owner":"framework"}`),
		CreatedAt:      time.Now().UTC(),
	}
	response, err := suspension.ValidateResponse(map[string]any{"approved": true})
	if err != nil {
		t.Fatalf("ValidateResponse: %v", err)
	}
	suspension.Response = response

	body, err := json.Marshal(suspension)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var decoded interaction.Suspension
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(decoded, suspension) {
		t.Fatalf("round trip = %#v, want %#v", decoded, suspension)
	}

	unknown := append(append([]byte(nil), body[:len(body)-1]...), []byte(`,"future":true}`)...)
	original := decoded
	if err := json.Unmarshal(unknown, &decoded); !errors.Is(err, interaction.ErrInvalidSuspension) {
		t.Fatalf("unknown field error = %v", err)
	}
	if !reflect.DeepEqual(decoded, original) {
		t.Fatal("unknown-field failure mutated receiver")
	}
}

func TestSuspensionRejectsSchemaMismatchAndInvalidWire(t *testing.T) {
	suspension := interaction.Suspension{
		SchemaVersion:  interaction.SuspensionSchemaVersion,
		ID:             "approval-1",
		Prompt:         json.RawMessage(`"approve?"`),
		ResponseSchema: json.RawMessage(`{"type":"boolean"}`),
		CreatedAt:      time.Now(),
	}
	if _, err := suspension.ValidateResponse("yes"); err == nil {
		t.Fatal("string response unexpectedly matched boolean schema")
	}
	invalid := suspension
	invalid.SchemaVersion = interaction.SuspensionSchemaVersion - 1
	if _, err := json.Marshal(invalid); !errors.Is(err, interaction.ErrInvalidSuspension) {
		t.Fatalf("invalid Marshal error = %v", err)
	}

	original := suspension
	if err := json.Unmarshal([]byte(`{"schema_version":0}`), &original); !errors.Is(err, interaction.ErrInvalidSuspension) {
		t.Fatalf("invalid Unmarshal error = %v", err)
	}
	if original.ID != suspension.ID {
		t.Fatal("failed Unmarshal mutated receiver")
	}

	responded := suspension
	responded.Response = json.RawMessage(`true`)
	if err := responded.Validate(); err != nil {
		t.Fatalf("valid response was rejected: %v", err)
	}

	invalidFrameworkState := suspension
	invalidFrameworkState.FrameworkState = json.RawMessage(`{`)
	if err := invalidFrameworkState.Validate(); !errors.Is(err, interaction.ErrInvalidSuspension) {
		t.Fatalf("invalid framework state error = %v", err)
	}
}
