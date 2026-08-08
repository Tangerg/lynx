package turn_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
)

func TestPreparedTurnDoesNotEnterEngineBeforeActivation(t *testing.T) {
	engine := &stubEngine{}
	controller, err := turn.New(turnDeps(engine))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	handle, err := controller.PrepareTurn(t.Context(), runs.StartExecution{
		SessionID: "session", Message: "hello",
	})
	if err != nil {
		t.Fatalf("PrepareTurn: %v", err)
	}
	if got := engine.runTurnCalls.Load(); got != 0 {
		t.Fatalf("engine calls before activation = %d, want 0", got)
	}

	events, err := controller.Events(context.Background(), handle)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if err := controller.ActivateTurn(t.Context(), handle); err != nil {
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
	controller, err := turn.New(turnDeps(engine))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	handle, err := controller.PrepareTurn(t.Context(), runs.StartExecution{
		SessionID: "session", Message: "hello",
	})
	if err != nil {
		t.Fatalf("PrepareTurn: %v", err)
	}
	events, err := controller.Events(context.Background(), handle)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if err := controller.Cancel(t.Context(), handle); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	var terminal runs.SegmentEnded
	for event := range events {
		if end, ok := event.Payload.(runs.SegmentEnded); ok {
			terminal = end
		}
	}
	if terminal.Reason != run.OutcomeCanceled {
		t.Fatalf("terminal reason = %q, want canceled", terminal.Reason)
	}
	if got := engine.runTurnCalls.Load(); got != 0 {
		t.Fatalf("engine calls after prepared cancel = %d, want 0", got)
	}
}
