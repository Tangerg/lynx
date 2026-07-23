package turn

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
)

type sessionTurnCanceler interface {
	Cancel(context.Context, TurnHandle) error
}

type sessionTurnCleanup struct{ dispatcher sessionTurnCanceler }

// NewSessionTurnCleanup adapts Agent turn cancellation to the narrow cleanup
// port consumed by the session lifecycle.
func NewSessionTurnCleanup(dispatcher sessionTurnCanceler) sessions.Turns {
	return sessionTurnCleanup{dispatcher: dispatcher}
}

func (t sessionTurnCleanup) Cancel(ctx context.Context, ref sessions.RunRef) error {
	err := t.dispatcher.Cancel(ctx, TurnHandle{SessionID: ref.SessionID, TurnID: ref.TurnID})
	if errors.Is(err, ErrTurnNotFound) {
		return nil
	}
	return err
}
