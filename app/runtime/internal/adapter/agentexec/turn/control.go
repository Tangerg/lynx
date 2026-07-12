package turn

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/catalog"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// SessionStore is the turn-control adapter's view of session persistence: resolve
// or create the session a turn runs in (Get / Create) and record the model a run
// explicitly selected (SetModel). Narrower than the sessions coordinator's
// lifecycle surface; the composition root threads the one sqlite-backed session
// store, which satisfies both.
type SessionStore interface {
	Get(ctx context.Context, id string) (session.Session, error)
	Create(ctx context.Context, title, cwd string) (session.Session, error)
	SetModel(ctx context.Context, id, model string) error
}

// Control is the turn-start use case: it plans a user turn draft against a
// persisted session, records an explicit model selection, and dispatches the
// turn. It lives in the adapter ring because it speaks the agent-SDK turn types
// ([StartTurnRequest] / [TurnHandle]); the delivery layer drives it and the run
// coordinator streams the dispatched turn through the [Executor]. Construct via
// [NewControl].
type Control struct {
	dispatcher Dispatcher
	sessions   SessionStore
}

// NewControl returns a Control over the turn dispatcher + session store.
func NewControl(dispatcher Dispatcher, sessions SessionStore) *Control {
	return &Control{dispatcher: dispatcher, sessions: sessions}
}

// PlanTurnStart validates a user turn draft and binds it to a persisted session.
// sessionID selects an existing session; empty creates a new one in defaultCwd.
// The returned request is ready for [Control.StartTurn].
func (c *Control) PlanTurnStart(ctx context.Context, sessionID, defaultCwd string, draft StartTurnRequest) (session.Session, StartTurnRequest, error) {
	if err := draft.Validate(); err != nil {
		return session.Session{}, StartTurnRequest{}, err
	}
	if len(draft.Media) > 0 && draft.Provider != "" && draft.Model != "" {
		if info, ok := catalog.Lookup(draft.Provider, draft.Model); ok && !info.Modalities.AcceptsInput(chat.ModalityImage) {
			return session.Session{}, StartTurnRequest{}, fmt.Errorf("%w: model %q (provider %q) does not accept image input", ErrUnsupportedMedia, draft.Model, draft.Provider)
		}
	}

	sess, err := c.resolveSession(ctx, sessionID, defaultCwd)
	if err != nil {
		return session.Session{}, StartTurnRequest{}, err
	}
	planned := draft
	planned.SessionID = sess.ID
	planned.Cwd = sess.Cwd
	return sess, planned, nil
}

func (c *Control) resolveSession(ctx context.Context, sessionID, defaultCwd string) (session.Session, error) {
	if sessionID == "" {
		return c.sessions.Create(ctx, "", defaultCwd)
	}
	return c.sessions.Get(ctx, sessionID)
}

// StartTurn launches one agent turn. An explicit model selection is recorded on
// the session before dispatch, so every caller gets the same session-model
// invariant.
func (c *Control) StartTurn(ctx context.Context, req StartTurnRequest) (TurnHandle, error) {
	if req.Model != "" {
		if err := c.sessions.SetModel(ctx, req.SessionID, req.Model); err != nil {
			return TurnHandle{}, err
		}
	}
	return c.dispatcher.StartTurn(ctx, req)
}

// InjectTurnSteering queues an in-flight steering message for a live turn.
func (c *Control) InjectTurnSteering(ctx context.Context, handle TurnHandle, message string) error {
	return c.dispatcher.InjectSteering(ctx, handle, message)
}
