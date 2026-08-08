package turn_test

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
)

func TestTurnInstallsChildAdmissionOnlyWhenExplicitlyEnabled(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		engine := &stubEngine{}
		controller, err := turn.New(turnDeps(engine))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		handle, err := controller.StartTurn(t.Context(), runs.RootExecutionStart{
			SessionID:                "session",
			Message:                  "hello",
			ChildRunAdmissionEnabled: enabled,
		})
		if err != nil {
			t.Fatalf("StartTurn enabled=%v: %v", enabled, err)
		}
		events, err := controller.Events(t.Context(), handle)
		if err != nil {
			t.Fatalf("Events enabled=%v: %v", enabled, err)
		}
		for range events {
		}
		if got := engine.childAdmissionEnabled(); got != enabled {
			t.Fatalf("child admission enabled=%v, want %v", got, enabled)
		}
	}
}
