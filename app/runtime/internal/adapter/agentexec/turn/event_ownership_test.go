package turn

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

func TestCancelBetweenParkAndInterruptPublishClosesSafely(t *testing.T) {
	release := make(chan struct{})
	close(release)
	st := newRunningTestState(
		t.Context(),
		Handle{SessionID: "ses_1", TurnID: "turn_1"},
		&blockingCancelProcess{release: release},
	)
	if !st.parkIfLive() {
		t.Fatal("failed to park test turn")
	}
	controller := &controller{
		turns:        map[string]*turnState{st.handle.TurnID: st},
		seenSessions: map[string]struct{}{},
	}

	if err := controller.Cancel(t.Context(), st.handle); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if controller.emitProcessEvent(st, agentexec.ProcessRef{ID: "proc_1"}, runs.TreeInterrupted{}) {
		t.Fatal("late interrupt was delivered after the terminal closed the stream")
	}

	var endCount int
	for ev := range st.events {
		if end, ok := ev.Payload.(runs.TurnEnd); ok {
			endCount++
			if end.Reason != execution.OutcomeCanceled {
				t.Fatalf("runs.TurnEnd reason = %s, want canceled", end.Reason)
			}
		}
	}
	if endCount != 1 {
		t.Fatalf("runs.TurnEnd count = %d, want 1", endCount)
	}
}
