package core

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
)

// MemoryProcessStore is the strict reference ProcessStore. It applies the
// same JSON boundary and CAS semantics expected from persistent backends.
type MemoryProcessStore struct {
	mu        sync.RWMutex
	snapshots map[string]ProcessSnapshot
}

// NewMemoryProcessStore returns an empty process store.
func NewMemoryProcessStore() *MemoryProcessStore {
	return &MemoryProcessStore{snapshots: map[string]ProcessSnapshot{}}
}

func (s *MemoryProcessStore) Save(_ context.Context, snapshot ProcessSnapshot, expectedRevision uint64) (uint64, error) {
	if s == nil {
		return 0, fmt.Errorf("memory process store: nil receiver")
	}
	if snapshot.Revision != expectedRevision {
		return 0, fmt.Errorf("%w: snapshot revision %d does not match expected revision %d", ErrInvalidSnapshot, snapshot.Revision, expectedRevision)
	}
	if err := snapshot.Validate(); err != nil {
		return 0, fmt.Errorf("memory process store: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	actualRevision := uint64(0)
	if current, ok := s.snapshots[snapshot.ID]; ok {
		actualRevision = current.Revision
	}
	if actualRevision != expectedRevision {
		return 0, &RevisionConflictError{ProcessID: snapshot.ID, Expected: expectedRevision, Actual: actualRevision}
	}
	snapshot.Revision = actualRevision + 1
	cloned, err := snapshot.clone()
	if err != nil {
		return 0, fmt.Errorf("memory process store: clone snapshot: %w", err)
	}
	s.snapshots[snapshot.ID] = cloned
	return snapshot.Revision, nil
}

func (s *MemoryProcessStore) Load(_ context.Context, id string) (ProcessSnapshot, error) {
	if s == nil {
		return ProcessSnapshot{}, fmt.Errorf("memory process store: nil receiver")
	}
	s.mu.RLock()
	snapshot, ok := s.snapshots[id]
	s.mu.RUnlock()
	if !ok {
		return ProcessSnapshot{}, fmt.Errorf("memory process store: load %q: %w", id, ErrSnapshotNotFound)
	}
	cloned, err := snapshot.clone()
	if err != nil {
		return ProcessSnapshot{}, fmt.Errorf("memory process store: clone loaded snapshot: %w", err)
	}
	if cloned.Revision == 0 {
		return ProcessSnapshot{}, fmt.Errorf("%w: stored revision is zero", ErrInvalidSnapshot)
	}
	return cloned, nil
}

func (s *MemoryProcessStore) Delete(_ context.Context, id string) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	delete(s.snapshots, id)
	s.mu.Unlock()
	return nil
}

func (s *MemoryProcessStore) List(_ context.Context) ([]string, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.RLock()
	ids := make([]string, 0, len(s.snapshots))
	for id := range s.snapshots {
		ids = append(ids, id)
	}
	s.mu.RUnlock()
	slices.Sort(ids)
	return ids, nil
}

func (s ProcessSnapshot) clone() (ProcessSnapshot, error) {
	data, err := json.Marshal(s)
	if err != nil {
		return ProcessSnapshot{}, err
	}
	var cloned ProcessSnapshot
	if err := json.Unmarshal(data, &cloned); err != nil {
		return ProcessSnapshot{}, err
	}
	return cloned, nil
}
