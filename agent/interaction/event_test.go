package interaction_test

import (
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
