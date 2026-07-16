package hitl

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/toolloop"
)

func TestIsInterruptUsesUnifiedSuspensionSignal(t *testing.T) {
	interrupt := &interaction.SuspendedError{Suspension: interaction.Suspension{
		SchemaVersion: interaction.SuspensionSchemaVersion,
		ID:            "approval",
		Kind:          interaction.SuspensionHuman,
		Prompt:        json.RawMessage(`"approve?"`),
		ResumeSchema:  json.RawMessage(`{"type":"boolean"}`),
		CreatedAt:     time.Now(),
	}}
	if !IsInterrupt(fmt.Errorf("wrapped: %w", interrupt)) {
		t.Fatal("wrapped suspension was not recognized")
	}
	if IsInterrupt(&toolloop.AbortError{Err: errors.New("fatal")}) {
		t.Fatal("ordinary tool-loop abort must not be treated as an interrupt")
	}
}
