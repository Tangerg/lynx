package core

import (
	"context"
	"fmt"
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
	snapshot, err := session.storageSnapshot()
	if err != nil {
		return fmt.Errorf("memory session store: snapshot %q: %w", session.ID, err)
	}
	return s.store.save(snapshot.ID, snapshot)
}

func (s *MemorySessionStore) Load(_ context.Context, id string) (Session, error) {
	stored, err := s.store.load(id)
	if err != nil {
		return Session{}, err
	}
	snapshot, err := stored.storageSnapshot()
	if err != nil {
		return Session{}, fmt.Errorf("memory session store: snapshot loaded session %q: %w", id, err)
	}
	return snapshot, nil
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
