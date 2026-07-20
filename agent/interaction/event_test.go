package interaction_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/core/chat"
)

func TestEventWrapsNestedProtocolValidation(t *testing.T) {
	event := interaction.Event{
		Kind:    interaction.EventModelRequest,
		Round:   1,
		Request: &chat.Request{},
	}
	if err := event.Validate(); !errors.Is(err, interaction.ErrInvalidEvent) {
		t.Fatalf("Validate error = %v, want ErrInvalidEvent", err)
	}
}

func TestValidateIDRejectsUnstableIdentity(t *testing.T) {
	for _, id := range []string{"", "   ", " approval-1", "approval-1 "} {
		if err := interaction.ValidateID(id); !errors.Is(err, interaction.ErrInvalidID) {
			t.Errorf("ValidateID(%q) error = %v", id, err)
		}
	}
	if err := interaction.ValidateID("approval-1"); err != nil {
		t.Fatalf("ValidateID: %v", err)
	}
}

func TestEventUnmarshalRejectsUnknownFields(t *testing.T) {
	body := []byte(`{"kind":"resume","round":1,"resume":{"id":"approval-1","input":true},"future":true}`)
	var event interaction.Event
	if err := json.Unmarshal(body, &event); !errors.Is(err, interaction.ErrInvalidEvent) {
		t.Fatalf("Unmarshal error = %v", err)
	}
}

func TestStopReasonValid(t *testing.T) {
	for _, reason := range []interaction.StopReason{interaction.StopNone, interaction.StopBudget, interaction.StopSteps} {
		if !reason.Valid() {
			t.Errorf("StopReason(%q) is invalid", reason)
		}
	}
	if interaction.StopReason("budget+steps").Valid() {
		t.Fatal("unknown stop reason is valid")
	}
}
