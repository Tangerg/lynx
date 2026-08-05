package runtime

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
)

func TestProcessStateSuspensionLifecycle(t *testing.T) {
	state := newProcessState()
	first := testSuspension("first")
	if err := state.parkSuspension(first); err != nil {
		t.Fatalf("park first: %v", err)
	}
	if err := state.parkSuspension(first); !errors.Is(err, interaction.ErrSuspensionConflict) {
		t.Fatalf("duplicate pending park error = %v", err)
	}
	if err := state.parkSuspension(testSuspension("other")); !errors.Is(err, interaction.ErrSuspensionConflict) {
		t.Fatalf("second pending park error = %v", err)
	}

	state.transition(core.StatusWaiting)
	if err := state.claimCheckpoint(false); err != nil {
		t.Fatal(err)
	}
	response, err := first.ValidateResponse(true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.installClaimedSuspensionResponse("first", response); err != nil {
		t.Fatalf("respond: %v", err)
	}
	if _, err := state.installClaimedSuspensionResponse("first", response); !errors.Is(err, interaction.ErrSuspensionStale) {
		t.Fatalf("duplicate response error = %v", err)
	}
	different, err := first.ValidateResponse(false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.installClaimedSuspensionResponse("first", different); !errors.Is(err, interaction.ErrSuspensionStale) {
		t.Fatalf("different response error = %v", err)
	}
	if _, err := state.installClaimedSuspensionResponse("other", response); !errors.Is(err, interaction.ErrSuspensionStale) {
		t.Fatalf("stale response error = %v", err)
	}
	state.releaseCheckpoint()

	second := testSuspension("second")
	if err := state.parkSuspension(second); err != nil {
		t.Fatalf("replace responded suspension: %v", err)
	}
	got := state.suspension()
	if got == nil || got.ID != "second" || got.Responded() {
		t.Fatalf("suspension = %#v", got)
	}
	got.Prompt[0] = 'x'
	if state.suspension().Prompt[0] == 'x' {
		t.Fatal("Suspension returned mutable process state")
	}
}

func TestSuspensionValidatesResponseSchema(t *testing.T) {
	if _, err := testSuspension("approval").ValidateResponse("yes"); err == nil {
		t.Fatal("string response unexpectedly matched boolean schema")
	}
}

func TestProcessStateTerminalTransitionClearsSuspension(t *testing.T) {
	state := newProcessState()
	if err := state.parkSuspension(testSuspension("approval")); err != nil {
		t.Fatal(err)
	}
	state.transition(core.StatusWaiting)
	if !state.markKilled(nil) {
		t.Fatal("kill did not win waiting process")
	}
	if state.suspension() != nil {
		t.Fatal("terminal transition retained suspension")
	}
}

func testSuspension(id string) interaction.Suspension {
	return interaction.Suspension{
		SchemaVersion:  interaction.SuspensionSchemaVersion,
		ID:             id,
		Prompt:         json.RawMessage(`"approve?"`),
		ResponseSchema: json.RawMessage(`{"type":"boolean"}`),
		CreatedAt:      time.Now(),
	}
}
