package core

import (
	"context"
	"fmt"
	"sync"
)

// InMemoryProcessStore is the reference [ProcessStore] backend —
// snapshots live in a goroutine-safe map. Suitable for tests and
// single-node development. Production deployments wire a persistent
// backend from the `agentstore/` sibling module.
type InMemoryProcessStore struct {
	mu        sync.RWMutex
	snapshots map[string]ProcessSnapshot
}

// NewInMemoryProcessStore returns an empty in-memory store ready for
// use.
func NewInMemoryProcessStore() *InMemoryProcessStore {
	return &InMemoryProcessStore{
		snapshots: map[string]ProcessSnapshot{},
	}
}

// Save inserts or overwrites the snapshot keyed by [ProcessSnapshot.ID].
// Empty id surfaces an error so callers don't silently lose snapshots
// to the zero key.
func (s *InMemoryProcessStore) Save(_ context.Context, snapshot ProcessSnapshot) error {
	if snapshot.ID == "" {
		return fmt.Errorf("in-memory process store: snapshot ID must not be empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshots[snapshot.ID] = snapshot
	return nil
}

// Load returns the snapshot stored under id, or [ErrSnapshotNotFound]
// when the id is unknown.
func (s *InMemoryProcessStore) Load(_ context.Context, id string) (ProcessSnapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snap, ok := s.snapshots[id]
	if !ok {
		return ProcessSnapshot{}, fmt.Errorf("in-memory process store: %w (id=%q)", ErrSnapshotNotFound, id)
	}
	return snap, nil
}

// Delete is idempotent — removing an unknown id is not an error.
func (s *InMemoryProcessStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.snapshots, id)
	return nil
}

// List returns every known process id. The order is map-iteration
// order — callers that need stability sort the result themselves.
func (s *InMemoryProcessStore) List(_ context.Context) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.snapshots))
	for id := range s.snapshots {
		ids = append(ids, id)
	}
	return ids, nil
}
