package turn

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
)

type sessionTurnCanceler interface {
	Cancel(ctx context.Context, handle Handle) error
}

// SessionExecutionCleanup adapts Agent turn cancellation to idempotent Session
// lifecycle cleanup.
type SessionExecutionCleanup struct{ controller sessionTurnCanceler }

// NewSessionExecutionCleanup adapts Agent turn cancellation to the narrow cleanup
// port consumed by the session lifecycle.
func NewSessionExecutionCleanup(controller sessionTurnCanceler) SessionExecutionCleanup {
	return SessionExecutionCleanup{controller: controller}
}

func (t SessionExecutionCleanup) Cancel(ctx context.Context, ref runs.ExecutorRef) error {
	err := t.controller.Cancel(ctx, Handle{SessionID: ref.SessionID, TurnID: ref.ExecutorID})
	if errors.Is(err, ErrTurnNotFound) {
		return nil
	}
	return err
}
