package bootstrap

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

type agentTurnCanceler struct {
	err error
}

func (c agentTurnCanceler) Cancel(context.Context, turn.Handle) error { return c.err }

func TestSessionExecutionCleanupTreatsMissingTurnAsIdempotent(t *testing.T) {
	cleanup := turn.NewSessionExecutionCleanup(agentTurnCanceler{err: turn.ErrTurnNotFound})
	if err := cleanup.Cancel(t.Context(), execution.ExecutorRef{SessionID: "ses_1", ExecutorID: "exec_1"}); err != nil {
		t.Fatalf("Cancel error = %v, want nil", err)
	}
}

func TestSessionExecutionCleanupPreservesFailure(t *testing.T) {
	want := errors.New("process cleanup failed")
	cleanup := turn.NewSessionExecutionCleanup(agentTurnCanceler{err: want})
	if err := cleanup.Cancel(t.Context(), execution.ExecutorRef{SessionID: "ses_1", ExecutorID: "exec_1"}); !errors.Is(err, want) {
		t.Fatalf("Cancel error = %v, want cleanup failure", err)
	}
}
