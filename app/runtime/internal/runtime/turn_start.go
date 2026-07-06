package runtime

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/catalog"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

// PlanTurnStart validates a user turn draft and binds it to a persisted
// session. sessionID selects an existing session; empty creates a new one in
// defaultCwd. The returned request is ready for StartTurn.
func (r *Runtime) PlanTurnStart(ctx context.Context, sessionID, defaultCwd string, draft turn.StartTurnRequest) (session.Session, turn.StartTurnRequest, error) {
	if err := draft.Validate(); err != nil {
		return session.Session{}, turn.StartTurnRequest{}, err
	}
	if len(draft.Media) > 0 && draft.Provider != "" && draft.Model != "" {
		if info, ok := catalog.Lookup(draft.Provider, draft.Model); ok && !info.Modalities.AcceptsInput(chat.ModalityImage) {
			return session.Session{}, turn.StartTurnRequest{}, fmt.Errorf("%w: model %q (provider %q) does not accept image input", turn.ErrUnsupportedMedia, draft.Model, draft.Provider)
		}
	}

	sess, err := r.resolveTurnSession(ctx, sessionID, defaultCwd)
	if err != nil {
		return session.Session{}, turn.StartTurnRequest{}, err
	}
	planned := draft
	planned.SessionID = sess.ID
	planned.Cwd = sess.Cwd
	return sess, planned, nil
}

func (r *Runtime) resolveTurnSession(ctx context.Context, sessionID, defaultCwd string) (session.Session, error) {
	if sessionID == "" {
		return r.session.Create(ctx, "", defaultCwd)
	}
	return r.session.Get(ctx, sessionID)
}
