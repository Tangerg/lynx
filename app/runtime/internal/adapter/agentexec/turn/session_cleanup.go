package turn

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

type sessionTurnCanceler interface {
	Cancel(context.Context, Handle) error
}

type sessionTurnCleanup struct{ controller sessionTurnCanceler }

// NewSessionTurnCleanup adapts Agent turn cancellation to the narrow cleanup
// port consumed by the session lifecycle.
func NewSessionTurnCleanup(controller sessionTurnCanceler) sessions.Turns {
	return sessionTurnCleanup{controller: controller}
}

func (t sessionTurnCleanup) Cancel(ctx context.Context, ref execution.TurnRef) error {
	err := t.controller.Cancel(ctx, Handle{SessionID: ref.SessionID, TurnID: ref.TurnID})
	if errors.Is(err, ErrTurnNotFound) {
		return nil
	}
	return err
}
