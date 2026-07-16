package core

import (
	"context"
	"slices"
)

// MemorySessionStore is the reference [SessionStore] backend —
// sessions live in a goroutine-safe map. Suitable for tests and
// single-node development. Production deployments supply their own
// persistent backend behind the same interface.
type MemorySessionStore struct {
	store *memoryStore[Session]
}

// NewMemorySessionStore returns an empty session store.
func NewMemorySessionStore() *MemorySessionStore {
	return &MemorySessionStore{
		store: newMemoryStore[Session]("memory session store", ErrSessionNotFound),
	}
}

func (s *MemorySessionStore) Save(_ context.Context, session Session) error {
	return s.store.save(session.ID, session)
}

func (s *MemorySessionStore) Load(_ context.Context, id string) (Session, error) {
	return s.store.load(id)
}

// Delete is idempotent.
func (s *MemorySessionStore) Delete(_ context.Context, id string) error {
	s.store.delete(id)
	return nil
}

// List returns every known session id in lexical order.
func (s *MemorySessionStore) List(_ context.Context) ([]string, error) {
	ids := s.store.list()
	slices.Sort(ids)
	return ids, nil
}
