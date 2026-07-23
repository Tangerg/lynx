package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	sessionsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

type childSessionPersistence interface {
	Get(ctx context.Context, id string) (sessionsvc.Session, error)
	SaveSubtask(ctx context.Context, subtask sessionsvc.Subtask) (sessionsvc.Session, error)
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
// SaveSubtask — an independent conversation (its own history) carrying the
// parent id, hidden from the user-facing session list.
type childSessionStore struct {
	sessions childSessionPersistence
}

var _ core.SessionStore = (*childSessionStore)(nil)

func newChildSessionStore(sessions childSessionPersistence) *childSessionStore {
	return &childSessionStore{sessions: sessions}
}

// Save records the spawned child session as a subtask session linked to its
// parent. The agent supplies the complete durable child identity; the product
// store enriches it with title and working directory inherited from the parent.
func (s *childSessionStore) Save(ctx context.Context, sess core.Session) error {
	if err := sess.Validate(); err != nil {
		return fmt.Errorf("child session store: save %q: %w", sess.ID, err)
	}
	metadata, err := json.Marshal(sess.Metadata)
	if err != nil {
		return fmt.Errorf("child session store: encode delegation metadata for %q: %w", sess.ID, err)
	}
	delegationMetadata, err := sessionsvc.ParseDelegationMetadata(metadata)
	if err != nil {
		return fmt.Errorf("child session store: validate delegation metadata for %q: %w", sess.ID, err)
	}
	_, err = s.sessions.SaveSubtask(ctx, sessionsvc.Subtask{
		ID:                 sess.ID,
		ParentID:           sess.ParentID,
		UserID:             sess.UserID,
		AgentName:          sess.AgentName,
		StartedAt:          sess.StartedAt,
		UpdatedAt:          sess.UpdatedAt,
		DelegationMetadata: delegationMetadata,
	})
	if err != nil {
		return fmt.Errorf("child session store: save %q: %w", sess.ID, err)
	}
	return nil
}

// Load returns the lineage-relevant fields of a stored session, mapping
// lyra's not-found to the SPI sentinel.
func (s *childSessionStore) Load(ctx context.Context, id string) (core.Session, error) {
	ls, err := s.sessions.Get(ctx, id)
	if errors.Is(err, sessionsvc.ErrNotFound) {
		return core.Session{}, fmt.Errorf("child session store: load %q: %w", id, core.ErrSessionNotFound)
	}
	if err != nil {
		return core.Session{}, fmt.Errorf("child session store: load %q: %w", id, err)
	}
	if ls.Kind != sessionsvc.KindSubtask {
		return core.Session{}, fmt.Errorf(
			"child session store: load %q: %w: stored kind %q",
			id,
			sessionsvc.ErrSubtaskConflict,
			ls.Kind,
		)
	}
	metadata, err := core.ParseSessionMetadata(ls.DelegationMetadata.JSON())
	if err != nil {
		return core.Session{}, fmt.Errorf("child session store: decode delegation metadata for %q: %w", id, err)
	}
	loaded := core.Session{
		ID:        ls.ID,
		ParentID:  ls.ParentID,
		UserID:    ls.UserID,
		AgentName: ls.AgentName,
		StartedAt: ls.StartedAt,
		UpdatedAt: ls.UpdatedAt,
		Metadata:  metadata,
	}
	if err := loaded.Validate(); err != nil {
		return core.Session{}, fmt.Errorf("child session store: load %q: %w", id, err)
	}
	return loaded, nil
}
