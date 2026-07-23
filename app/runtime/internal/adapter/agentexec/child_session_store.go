package agentexec

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

// NewChildSessionStore adapts product-session persistence to the Agent runtime
// session SPI. It persists only delegated child sessions; the Agent continuation
// remains an opaque sidecar outside the product Session domain.
func NewChildSessionStore(sessions childSessionPersistence) core.SessionStore {
	return &childSessionStore{sessions: sessions}
}

type childSessionStore struct{ sessions childSessionPersistence }

var _ core.SessionStore = (*childSessionStore)(nil)

func (s *childSessionStore) Save(ctx context.Context, value core.Session) error {
	if err := value.Validate(); err != nil {
		return fmt.Errorf("child session store: save %q: %w", value.ID, err)
	}
	if _, encoded, err := s.sessions.LoadSubtask(ctx, value.ID); err == nil {
		stored, decodeErr := decodeAgentSession(encoded)
		if decodeErr != nil {
			return fmt.Errorf("child session store: decode stored session %q: %w", value.ID, decodeErr)
		}
		if !sameAgentSessionIdentity(stored, value) {
			return fmt.Errorf("child session store: save %q: %w", value.ID, sessionsvc.ErrSubtaskConflict)
		}
	} else if !errors.Is(err, sessionsvc.ErrNotFound) {
		return fmt.Errorf("child session store: inspect existing session %q: %w", value.ID, err)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("child session store: encode agent session %q: %w", value.ID, err)
	}
	_, err = s.sessions.SaveSubtask(ctx, sessionsvc.Subtask{
		ID:        value.ID,
		ParentID:  value.ParentID,
		StartedAt: value.StartedAt,
		UpdatedAt: value.UpdatedAt,
	}, encoded)
	if err != nil {
		return fmt.Errorf("child session store: save %q: %w", value.ID, err)
	}
	return nil
}

func (s *childSessionStore) Load(ctx context.Context, id string) (core.Session, error) {
	product, encoded, err := s.sessions.LoadSubtask(ctx, id)
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
	if loaded.ID != product.ID || loaded.ParentID != product.ParentID ||
		!loaded.StartedAt.Equal(product.StartedAt) || !loaded.UpdatedAt.Equal(product.UpdatedAt) {
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

func sameAgentSessionIdentity(existing, next core.Session) bool {
	return existing.ID == next.ID &&
		existing.ParentID == next.ParentID &&
		existing.UserID == next.UserID &&
		existing.AgentName == next.AgentName &&
		existing.StartedAt.Equal(next.StartedAt)
}
