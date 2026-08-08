package bootstrap

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
)

type agentTurnCanceler struct {
	err error
}

func (c agentTurnCanceler) Cancel(context.Context, turn.Handle) error { return c.err }

func TestSessionExecutionReleaserTreatsMissingTurnAsIdempotent(t *testing.T) {
	cleanup := turn.NewSessionExecutionReleaser(agentTurnCanceler{err: turn.ErrTurnNotFound})
	if err := cleanup.Release(t.Context(), runs.ExecutorRef{SessionID: "ses_1", ExecutorID: "exec_1"}); err != nil {
		t.Fatalf("Cancel error = %v, want nil", err)
	}
}

func TestSessionExecutionReleaserPreservesFailure(t *testing.T) {
	want := errors.New("process cleanup failed")
	cleanup := turn.NewSessionExecutionReleaser(agentTurnCanceler{err: want})
	if err := cleanup.Release(t.Context(), runs.ExecutorRef{SessionID: "ses_1", ExecutorID: "exec_1"}); !errors.Is(err, want) {
		t.Fatalf("Cancel error = %v, want cleanup failure", err)
	}
}
