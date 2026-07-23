package bootstrap

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
)

type cancelDispatcher struct {
	err error
}

func (d cancelDispatcher) Cancel(context.Context, turn.TurnHandle) error { return d.err }

func TestSessionsTurnsTreatsMissingTurnAsIdempotentCleanup(t *testing.T) {
	adapter := turn.NewSessionTurnCleanup(cancelDispatcher{err: turn.ErrTurnNotFound})
	if err := adapter.Cancel(t.Context(), sessions.RunRef{SessionID: "ses_1", TurnID: "turn_1"}); err != nil {
		t.Fatalf("Cancel error = %v, want nil", err)
	}
}

func TestSessionsTurnsPreservesCleanupFailure(t *testing.T) {
	want := errors.New("process cleanup failed")
	adapter := turn.NewSessionTurnCleanup(cancelDispatcher{err: want})
	if err := adapter.Cancel(t.Context(), sessions.RunRef{SessionID: "ses_1", TurnID: "turn_1"}); !errors.Is(err, want) {
		t.Fatalf("Cancel error = %v, want cleanup failure", err)
	}
}
