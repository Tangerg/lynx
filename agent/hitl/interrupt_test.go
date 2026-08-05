package hitl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/toolloop"
)

type suspensionRecorder struct {
	called     bool
	suspension interaction.Suspension
}

func (*suspensionRecorder) Terminate(string) {}
func (*suspensionRecorder) CancelToolCall()  {}
func (r *suspensionRecorder) Suspend(_ context.Context, suspension interaction.Suspension) (core.ActionStatus, error) {
	r.called = true
	r.suspension = suspension
	return core.ActionWaiting, nil
}

func TestIsSuspendedUsesUnifiedSuspensionSignal(t *testing.T) {
	interrupt := &interaction.SuspendedError{Suspension: interaction.Suspension{
		SchemaVersion:  interaction.SuspensionSchemaVersion,
		ID:             "approval",
		Prompt:         json.RawMessage(`"approve?"`),
		ResponseSchema: json.RawMessage(`{"type":"boolean"}`),
		CreatedAt:      time.Now(),
	}}
	if !IsSuspended(fmt.Errorf("wrapped: %w", interrupt)) {
		t.Fatal("wrapped suspension was not recognized")
	}
	if IsSuspended(&toolloop.AbortError{Err: errors.New("fatal")}) {
		t.Fatal("ordinary tool-loop abort must not be treated as an interrupt")
	}
}

func TestHandleSuspensionParksAtUntypedActionBoundary(t *testing.T) {
	interrupt := &interaction.SuspendedError{Suspension: interaction.Suspension{
		SchemaVersion:  interaction.SuspensionSchemaVersion,
		ID:             "approval",
		Prompt:         json.RawMessage(`"approve?"`),
		ResponseSchema: json.RawMessage(`{"type":"boolean"}`),
		CreatedAt:      time.Now(),
	}}
	recorder := new(suspensionRecorder)
	process := core.NewProcessContext(core.ProcessContextConfig{Control: recorder})

	status, handled, err := HandleSuspension(t.Context(), process, fmt.Errorf("wrapped: %w", interrupt))
	if err != nil {
		t.Fatalf("HandleSuspension: %v", err)
	}
	if !handled || status != core.ActionWaiting || !recorder.called || recorder.suspension.ID != "approval" {
		t.Fatalf("HandleSuspension = status %s handled %v recorder %#v", status, handled, recorder)
	}

	status, handled, err = HandleSuspension(t.Context(), process, errors.New("ordinary"))
	if err != nil || handled || status != 0 {
		t.Fatalf("ordinary error = status %s handled %v err %v", status, handled, err)
	}
}
