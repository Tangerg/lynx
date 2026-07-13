package bootstrap

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
)

// sessionsTurns adapts the agent turn dispatcher to the session lifecycle's
// narrow parked-process cleanup port. Complete run commands use turn.Executor
// through application/runs instead.
type sessionsTurns struct {
	dispatcher turn.Dispatcher
}

func (t sessionsTurns) Cancel(ctx context.Context, ref sessions.RunRef) error {
	return t.dispatcher.Cancel(ctx, turn.TurnHandle{SessionID: ref.SessionID, TurnID: ref.TurnID})
}
