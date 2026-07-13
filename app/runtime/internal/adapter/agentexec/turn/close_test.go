package turn

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCloseIsBoundedAndCanFinishJoiningLater(t *testing.T) {
	st := newTurnState(t.Context(), TurnHandle{SessionID: "ses_1", TurnID: "turn_1"})
	svc := &inMemory{
		turns:        map[string]*turnState{st.handle.TurnID: st},
		seenSessions: map[string]struct{}{},
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	err := svc.close(ctx)
	if !errors.Is(err, ErrCloseTimeout) {
		t.Fatalf("close error = %v, want ErrCloseTimeout", err)
	}
	if !svc.isClosed() {
		t.Fatal("timed-out close did not reject future admission")
	}

	close(st.done)
	if err := svc.close(t.Context()); err != nil {
		t.Fatalf("second close after teardown = %v, want nil", err)
	}
}
