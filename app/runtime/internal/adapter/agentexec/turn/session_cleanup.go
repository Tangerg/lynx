package turn

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

type sessionTurnCanceler interface {
	Cancel(context.Context, Handle) error
}

// SessionTurnCleanup adapts Agent turn cancellation to idempotent session
// lifecycle cleanup.
type SessionTurnCleanup struct{ controller sessionTurnCanceler }

// NewSessionTurnCleanup adapts Agent turn cancellation to the narrow cleanup
// port consumed by the session lifecycle.
func NewSessionTurnCleanup(controller sessionTurnCanceler) SessionTurnCleanup {
	return SessionTurnCleanup{controller: controller}
}

func (t SessionTurnCleanup) Cancel(ctx context.Context, ref execution.ExecutorRef) error {
	err := t.controller.Cancel(ctx, Handle{SessionID: ref.SessionID, TurnID: ref.ExecutorID})
	if errors.Is(err, ErrTurnNotFound) {
		return nil
	}
	return err
}
