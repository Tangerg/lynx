package core

import "context"

// InMemorySessionStore is the reference [SessionStore] backend —
// sessions live in a goroutine-safe map. Suitable for tests and
// single-node development. Production deployments supply their own
// persistent backend behind the same interface.
type InMemorySessionStore struct {
	kv *inMemoryKV[Session]
}

func NewInMemorySessionStore() *InMemorySessionStore {
	return &InMemorySessionStore{
		kv: newInMemoryKV[Session]("in-memory session store", ErrSessionNotFound),
	}
}

func (s *InMemorySessionStore) Save(_ context.Context, session Session) error {
	return s.kv.save(session.ID, session)
}

func (s *InMemorySessionStore) Load(_ context.Context, id string) (Session, error) {
	return s.kv.load(id)
}

// Delete is idempotent.
func (s *InMemorySessionStore) Delete(_ context.Context, id string) error {
	s.kv.delete(id)
	return nil
}

// List returns every known session id in map-iteration order.
func (s *InMemorySessionStore) List(_ context.Context) ([]string, error) {
	return s.kv.list(), nil
}
