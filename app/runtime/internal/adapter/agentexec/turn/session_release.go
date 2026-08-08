package turn

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
)

type sessionTurnCanceler interface {
	Cancel(ctx context.Context, handle Handle) error
}

// SessionExecutionReleaser adapts Agent turn cancellation to idempotent Session
// resource release.
type SessionExecutionReleaser struct{ controller sessionTurnCanceler }

// NewSessionExecutionReleaser adapts Agent turn cancellation to the narrow
// resource-release port consumed by the Session lifecycle.
func NewSessionExecutionReleaser(controller sessionTurnCanceler) SessionExecutionReleaser {
	return SessionExecutionReleaser{controller: controller}
}

// Release tears down the legacy Agent turn and treats an already absent owner
// as an idempotent resource-lifecycle success.
func (t SessionExecutionReleaser) Release(ctx context.Context, ref runs.ExecutorRef) error {
	err := t.controller.Cancel(ctx, Handle{SessionID: ref.SessionID, TurnID: ref.ExecutorID})
	if errors.Is(err, ErrTurnNotFound) {
		return nil
	}
	return err
}
