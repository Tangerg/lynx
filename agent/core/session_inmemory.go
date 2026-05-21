package core

import (
	"context"
	"fmt"
	"sync"
)

// InMemorySessionStore is the reference [SessionStore] backend —
// sessions live in a goroutine-safe map. Suitable for tests and
// single-node development. Production deployments wire a persistent
// backend from the `agentstore/` sibling module.
type InMemorySessionStore struct {
	mu       sync.RWMutex
	sessions map[string]Session
}

// NewInMemorySessionStore returns an empty store ready for use.
func NewInMemorySessionStore() *InMemorySessionStore {
	return &InMemorySessionStore{sessions: map[string]Session{}}
}

// Save inserts or overwrites the session keyed by [Session.ID].
// Empty id is rejected so callers don't silently lose sessions to
// the zero key.
func (s *InMemorySessionStore) Save(_ context.Context, session Session) error {
	if session.ID == "" {
		return fmt.Errorf("in-memory session store: session ID must not be empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.ID] = session
	return nil
}

// Load returns the session stored under id, wrapping
// [ErrSessionNotFound] for unknown ids.
func (s *InMemorySessionStore) Load(_ context.Context, id string) (Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	if !ok {
		return Session{}, fmt.Errorf("in-memory session store: %w (id=%q)", ErrSessionNotFound, id)
	}
	return sess, nil
}

// Delete is idempotent.
func (s *InMemorySessionStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
	return nil
}

// List returns every known session id in map-iteration order.
func (s *InMemorySessionStore) List(_ context.Context) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.sessions))
	for id := range s.sessions {
		ids = append(ids, id)
	}
	return ids, nil
}
