package turn_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

func TestPreparedTurnDoesNotEnterEngineBeforeActivation(t *testing.T) {
	engine := &stubEngine{}
	dispatcher, err := turn.New(turnDeps(engine))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	handle, err := dispatcher.PrepareTurn(t.Context(), turn.StartTurnRequest{
		SessionID: "session", Message: "hello",
	})
	if err != nil {
		t.Fatalf("PrepareTurn: %v", err)
	}
	if got := engine.runTurnCalls.Load(); got != 0 {
		t.Fatalf("engine calls before activation = %d, want 0", got)
	}

	events, err := dispatcher.Events(context.Background(), handle)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if err := dispatcher.ActivateTurn(t.Context(), handle); err != nil {
		t.Fatalf("ActivateTurn: %v", err)
	}
	for range events {
	}
	if got := engine.runTurnCalls.Load(); got != 1 {
		t.Fatalf("engine calls after activation = %d, want 1", got)
	}
}

func TestCancelPreparedTurnNeverEntersEngineAndTerminatesStream(t *testing.T) {
	engine := &stubEngine{}
	dispatcher, err := turn.New(turnDeps(engine))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	handle, err := dispatcher.PrepareTurn(t.Context(), turn.StartTurnRequest{
		SessionID: "session", Message: "hello",
	})
	if err != nil {
		t.Fatalf("PrepareTurn: %v", err)
	}
	events, err := dispatcher.Events(context.Background(), handle)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if err := dispatcher.Cancel(t.Context(), handle); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	var terminal runs.TurnEnd
	for event := range events {
		if end, ok := event.(runs.TurnEnd); ok {
			terminal = end
		}
	}
	if terminal.Reason != execution.OutcomeCanceled {
		t.Fatalf("terminal reason = %q, want canceled", terminal.Reason)
	}
	if got := engine.runTurnCalls.Load(); got != 0 {
		t.Fatalf("engine calls after prepared cancel = %d, want 0", got)
	}
}
