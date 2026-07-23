package bootstrap

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
)

// sessionsTurns adapts the agent turn dispatcher to the session lifecycle's
// narrow parked-process cleanup port. Complete run commands use turn.Executor
// through application/runs instead.
type sessionsTurns struct {
	dispatcher turnCanceler
}

type turnCanceler interface {
	Cancel(context.Context, turn.TurnHandle) error
}

func (t sessionsTurns) Cancel(ctx context.Context, ref sessions.RunRef) error {
	err := t.dispatcher.Cancel(ctx, turn.TurnHandle{SessionID: ref.SessionID, TurnID: ref.TurnID})
	if errors.Is(err, turn.ErrTurnNotFound) {
		return nil
	}
	return err
}
