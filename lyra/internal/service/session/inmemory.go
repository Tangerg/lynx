package session

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// NewInMemoryService returns a [Service] backed by an in-process map.
// Suitable for the M3 walking-skeleton and for tests; durable
// backends (sqlite / postgres / file-based JSONL) arrive later.
//
// All methods are safe for concurrent use.
func NewInMemoryService() Service {
	return &inMemoryService{sessions: map[string]*Session{}}
}

type inMemoryService struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

func (s *inMemoryService) List(_ context.Context) ([]Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		out = append(out, *sess)
	}
	return out, nil
}

func (s *inMemoryService) Get(_ context.Context, id string) (Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sess, ok := s.sessions[id]
	if !ok {
		return Session{}, ErrNotFound
	}
	return *sess, nil
}

func (s *inMemoryService) Create(_ context.Context, title string) (Session, error) {
	now := time.Now().UTC()
	sess := &Session{
		ID:        uuid.NewString(),
		Title:     title,
		StartedAt: now,
		UpdatedAt: now,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sess.ID] = sess
	return *sess, nil
}

// Fork is a structural operation in M3 — it duplicates the parent's
// identity into a child session pointing at the parent via
// ParentID. The actual conversation rewind (replaying parent history
// up to atMessageID before diverging) lives in the chat-memory layer
// and lands when the chatmemory backend grows a fork-aware Save (M5+
// scope). For now atMessageID is recorded as metadata so callers
// don't lose the intent.
func (s *inMemoryService) Fork(_ context.Context, parentID, atMessageID string) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	parent, ok := s.sessions[parentID]
	if !ok {
		return Session{}, ErrNotFound
	}

	now := time.Now().UTC()
	child := &Session{
		ID:        uuid.NewString(),
		Title:     parent.Title + " (fork)",
		ParentID:  parentID,
		StartedAt: now,
		UpdatedAt: now,
		Metadata: map[string]string{
			"fork_at_message_id": atMessageID,
		},
	}
	s.sessions[child.ID] = child
	return *child, nil
}

func (s *inMemoryService) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
	return nil
}

// Touch updates a session's UpdatedAt + bumps TurnCount. Internal —
// called by ChatService at turn end so List orders correctly by
// recency. Returns ErrNotFound when the session was deleted while a
// turn was in flight.
//
// Lives on *inMemoryService rather than [Service] because it's
// implementation-detail bookkeeping, not part of the public surface
// that transport adapters expose.
func (s *inMemoryService) Touch(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return ErrNotFound
	}
	sess.UpdatedAt = time.Now().UTC()
	sess.TurnCount++
	return nil
}
