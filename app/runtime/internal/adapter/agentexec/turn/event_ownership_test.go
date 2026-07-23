package turn

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

func TestCancelBetweenParkAndInterruptPublishClosesSafely(t *testing.T) {
	st := newTurnState(t.Context(), TurnHandle{SessionID: "ses_1", TurnID: "turn_1"})
	if !st.parkIfLive() {
		t.Fatal("failed to park test turn")
	}
	dispatcher := &memoryDispatcher{
		turns:        map[string]*turnState{st.handle.TurnID: st},
		seenSessions: map[string]struct{}{},
	}

	if err := dispatcher.Cancel(t.Context(), st.handle); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if dispatcher.emit(st, runs.TurnInterrupted{}) {
		t.Fatal("late interrupt was delivered after the terminal closed the stream")
	}

	var endCount int
	for ev := range st.events {
		if end, ok := ev.(runs.TurnEnd); ok {
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
