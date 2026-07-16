package bootstrap

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/agent/core"
	sessionsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

type childSessionPersistence interface {
	List(ctx context.Context) ([]sessionsvc.Session, error)
	Get(ctx context.Context, id string) (sessionsvc.Session, error)
	CreateSubtask(ctx context.Context, id, parentID string) (sessionsvc.Session, error)
	Delete(ctx context.Context, id string) error
}

// childSessionStore adapts lyra's [sessionsvc.Store] to the agent runtime's
// [core.SessionStore] SPI. Wiring it into the agent engine makes the runtime
// persist a sub-agent's session when it spawns one (the `task` delegation), so
// the parent→child lineage is durably queryable on the lyra side rather than
// living only in the agent's in-process ProcessOptions.
//
// Only CHILD sessions reach this adapter: lyra drives parent turns through the
// engine directly rather than through RunInSession, so the only Save calls come
// from the spawn path. Each is mapped to a [sessionsvc.KindSubtask] session via
// CreateSubtask — an independent conversation (its own history) carrying the
// parent id, hidden from the user-facing session list.
type childSessionStore struct {
	sessions childSessionPersistence
}

var _ core.SessionStore = (*childSessionStore)(nil)

func newChildSessionStore(sessions childSessionPersistence) *childSessionStore {
	return &childSessionStore{sessions: sessions}
}

// Save records the spawned child session as a subtask session linked to its
// parent. The agent supplies the child id (its conversation id) and ParentID;
// the working directory is inherited from the parent inside CreateSubtask.
func (s *childSessionStore) Save(ctx context.Context, sess core.Session) error {
	_, err := s.sessions.CreateSubtask(ctx, sess.ID, sess.ParentID)
	return err
}

// Load returns the lineage-relevant fields of a stored session, mapping
// lyra's not-found to the SPI sentinel.
func (s *childSessionStore) Load(ctx context.Context, id string) (core.Session, error) {
	ls, err := s.sessions.Get(ctx, id)
	if errors.Is(err, sessionsvc.ErrNotFound) {
		return core.Session{}, core.ErrSessionNotFound
	}
	if err != nil {
		return core.Session{}, err
	}
	return core.Session{
		ID:        ls.ID,
		ParentID:  ls.ParentID,
		StartedAt: ls.StartedAt,
		UpdatedAt: ls.UpdatedAt,
	}, nil
}

// Delete drops the session by id (idempotent, per both contracts).
func (s *childSessionStore) Delete(ctx context.Context, id string) error {
	return s.sessions.Delete(ctx, id)
}

// List returns the user-facing session ids. Subtask children are hidden by
// lyra's List by design; the agent runtime does not call List in lyra's flow.
func (s *childSessionStore) List(ctx context.Context) ([]string, error) {
	sessions, err := s.sessions.List(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(sessions))
	for i, sess := range sessions {
		ids[i] = sess.ID
	}
	return ids, nil
}
