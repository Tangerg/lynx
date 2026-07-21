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
	if err := state.parkSuspension(first); err != nil {
		t.Fatalf("idempotent park: %v", err)
	}
	if err := state.parkSuspension(testSuspension("other")); !errors.Is(err, interaction.ErrSuspensionConflict) {
		t.Fatalf("second pending park error = %v", err)
	}

	state.transition(core.StatusWaiting)
	answeredAt := time.Now()
	if err := state.respondToSuspension("first", true, answeredAt); err != nil {
		t.Fatalf("respond: %v", err)
	}
	if err := state.respondToSuspension("first", true, answeredAt); err != nil {
		t.Fatalf("idempotent response: %v", err)
	}
	if err := state.respondToSuspension("first", false, answeredAt); !errors.Is(err, interaction.ErrSuspensionConflict) {
		t.Fatalf("different response error = %v", err)
	}
	if err := state.respondToSuspension("other", true, answeredAt); !errors.Is(err, interaction.ErrSuspensionStale) {
		t.Fatalf("stale response error = %v", err)
	}

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

func TestProcessStateSuspensionValidatesResponseSchema(t *testing.T) {
	state := newProcessState()
	if err := state.parkSuspension(testSuspension("approval")); err != nil {
		t.Fatal(err)
	}
	state.transition(core.StatusWaiting)
	if err := state.respondToSuspension("approval", "yes", time.Now()); err == nil {
		t.Fatal("string response unexpectedly matched boolean schema")
	}
}

func TestProcessStateTerminalTransitionClearsSuspension(t *testing.T) {
	state := newProcessState()
	if err := state.parkSuspension(testSuspension("approval")); err != nil {
		t.Fatal(err)
	}
	state.transition(core.StatusWaiting)
	state.pauseDurability()
	if state.status() != core.StatusWaiting || state.suspension() == nil {
		t.Fatalf("durability pause changed waiting continuation: status=%s suspension=%#v", state.status(), state.suspension())
	}
	if won, _ := state.markKilled(nil); !won {
		t.Fatal("kill did not win waiting process")
	}
	if state.suspension() != nil {
		t.Fatal("terminal transition retained suspension")
	}
}

func testSuspension(id string) interaction.Suspension {
	return interaction.Suspension{
		SchemaVersion: interaction.SuspensionSchemaVersion,
		ID:            id,
		Kind:          interaction.SuspensionHuman,
		Prompt:        json.RawMessage(`"approve?"`),
		ResumeSchema:  json.RawMessage(`{"type":"boolean"}`),
		CreatedAt:     time.Now(),
	}
}
