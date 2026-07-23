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
	LoadSubtask(ctx context.Context, id string) (sessionsvc.Session, []byte, error)
	SaveSubtask(ctx context.Context, subtask sessionsvc.Subtask, agentSession []byte) (sessionsvc.Session, error)
}

// childSessionStore adapts Lyra product-session persistence to the agent
// runtime's [core.SessionStore] SPI. Wiring it into the agent engine makes the runtime
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
// parent. The agent session is serialized as an opaque Bootstrap-owned sidecar;
// the product store receives only its lineage/audit projection and enriches it
// with title and working directory inherited from the parent.
func (s *childSessionStore) Save(ctx context.Context, sess core.Session) error {
	if err := sess.Validate(); err != nil {
		return fmt.Errorf("child session store: save %q: %w", sess.ID, err)
	}
	if _, encoded, err := s.sessions.LoadSubtask(ctx, sess.ID); err == nil {
		stored, decodeErr := decodeAgentSession(encoded)
		if decodeErr != nil {
			return fmt.Errorf("child session store: decode stored session %q: %w", sess.ID, decodeErr)
		}
		if !sameAgentSessionIdentity(stored, sess) {
			return fmt.Errorf("child session store: save %q: %w", sess.ID, sessionsvc.ErrSubtaskConflict)
		}
	} else if !errors.Is(err, sessionsvc.ErrNotFound) {
		return fmt.Errorf("child session store: inspect existing session %q: %w", sess.ID, err)
	}
	encoded, err := json.Marshal(sess)
	if err != nil {
		return fmt.Errorf("child session store: encode agent session %q: %w", sess.ID, err)
	}
	_, err = s.sessions.SaveSubtask(ctx, sessionsvc.Subtask{
		ID:        sess.ID,
		ParentID:  sess.ParentID,
		StartedAt: sess.StartedAt,
		UpdatedAt: sess.UpdatedAt,
	}, encoded)
	if err != nil {
		return fmt.Errorf("child session store: save %q: %w", sess.ID, err)
	}
	return nil
}

// Load restores the complete agent runtime session from Bootstrap's opaque
// sidecar, mapping product-store not-found to the SPI sentinel.
func (s *childSessionStore) Load(ctx context.Context, id string) (core.Session, error) {
	ls, encoded, err := s.sessions.LoadSubtask(ctx, id)
	if errors.Is(err, sessionsvc.ErrNotFound) {
		return core.Session{}, fmt.Errorf("child session store: load %q: %w", id, core.ErrSessionNotFound)
	}
	if err != nil {
		return core.Session{}, fmt.Errorf("child session store: load %q: %w", id, err)
	}
	loaded, err := decodeAgentSession(encoded)
	if err != nil {
		return core.Session{}, fmt.Errorf("child session store: decode agent session %q: %w", id, err)
	}
	if loaded.ID != ls.ID || loaded.ParentID != ls.ParentID ||
		!loaded.StartedAt.Equal(ls.StartedAt) || !loaded.UpdatedAt.Equal(ls.UpdatedAt) {
		return core.Session{}, fmt.Errorf("child session store: load %q: %w: product and agent session state disagree", id, sessionsvc.ErrSubtaskConflict)
	}
	return loaded, nil
}

func decodeAgentSession(encoded []byte) (core.Session, error) {
	var loaded core.Session
	if err := json.Unmarshal(encoded, &loaded); err != nil {
		return core.Session{}, err
	}
	if err := loaded.Validate(); err != nil {
		return core.Session{}, err
	}
	return loaded, nil
}

// sameAgentSessionIdentity preserves the Agent runtime's immutable identity
// contract without making that contract a Session-domain concern. UpdatedAt and
// Metadata are expected to evolve on continuation saves.
func sameAgentSessionIdentity(existing, next core.Session) bool {
	return existing.ID == next.ID &&
		existing.ParentID == next.ParentID &&
		existing.UserID == next.UserID &&
		existing.AgentName == next.AgentName &&
		existing.StartedAt.Equal(next.StartedAt)
}
