package core

import "context"

// InMemoryProcessStore is the reference [ProcessStore] backend —
// snapshots live in a goroutine-safe map. Suitable for tests and
// single-node development. Production deployments supply their own
// persistent backend behind the same interface.
type InMemoryProcessStore struct {
	kv *inMemoryKV[ProcessSnapshot]
}

// NewInMemoryProcessStore returns an empty in-memory store ready for use.
func NewInMemoryProcessStore() *InMemoryProcessStore {
	return &InMemoryProcessStore{
		kv: newInMemoryKV[ProcessSnapshot]("in-memory process store", ErrSnapshotNotFound),
	}
}

func (s *InMemoryProcessStore) Save(_ context.Context, snapshot ProcessSnapshot) error {
	return s.kv.save(snapshot.ID, snapshot)
}

func (s *InMemoryProcessStore) Load(_ context.Context, id string) (ProcessSnapshot, error) {
	return s.kv.load(id)
}

// Delete is idempotent — removing an unknown id is not an error.
func (s *InMemoryProcessStore) Delete(_ context.Context, id string) error {
	s.kv.delete(id)
	return nil
}

// List returns every known process id in map-iteration order.
func (s *InMemoryProcessStore) List(_ context.Context) ([]string, error) {
	return s.kv.list(), nil
}
