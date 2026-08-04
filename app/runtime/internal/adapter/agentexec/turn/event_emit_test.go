package turn

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
)

func TestEmitProcessEventPreservesExecutorIdentity(t *testing.T) {
	state := newTurnState(
		t.Context(),
		Handle{SessionID: "session_1", TurnID: "turn_1"},
		turnRunning,
	)
	t.Cleanup(state.cancel)

	process := agentexec.ProcessRef{
		ID:          "process_child",
		ParentID:    "process_root",
		SpawnCallID: "call_delegate",
	}
	payload := runs.MessageDelta{Text: "delegated output"}
	controller := new(controller)
	if !controller.emitProcessEvent(state, process, payload) {
		t.Fatal("emitProcessEvent() = false, want delivered event")
	}

	event := <-state.events
	wantSource := runs.ExecutorSource{
		ProcessID:   process.ID,
		ParentID:    process.ParentID,
		SpawnCallID: process.SpawnCallID,
	}
	if event.Source != wantSource {
		t.Fatalf("event source = %+v, want %+v", event.Source, wantSource)
	}
	if event.Payload != payload {
		t.Fatalf("event payload = %#v, want %#v", event.Payload, payload)
	}
}
