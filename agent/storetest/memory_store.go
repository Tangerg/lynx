package storetest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"sync"

	"github.com/Tangerg/lynx/agent/core"
)

// MemoryProcessStore is a strict in-memory test double for [core.ProcessStore].
// It applies the JSON ownership boundary and compare-and-swap semantics that
// production adapters must provide.
type MemoryProcessStore struct {
	mu        sync.RWMutex
	snapshots map[string]core.ProcessSnapshot
}

var _ core.SnapshotBatchWriter = (*MemoryProcessStore)(nil)

// NewMemoryProcessStore returns an empty process-store test double.
func NewMemoryProcessStore() *MemoryProcessStore {
	return &MemoryProcessStore{snapshots: make(map[string]core.ProcessSnapshot)}
}

// Save implements [core.SnapshotWriter].
func (s *MemoryProcessStore) Save(ctx context.Context, snapshot core.ProcessSnapshot) error {
	return s.SaveBatch(ctx, []core.ProcessSnapshot{snapshot})
}

// SaveBatch implements [core.SnapshotBatchWriter].
func (s *MemoryProcessStore) SaveBatch(ctx context.Context, snapshots []core.ProcessSnapshot) error {
	if s == nil {
		return errors.New("storetest.MemoryProcessStore: nil receiver")
	}
	if err := contextError(ctx); err != nil {
		return fmt.Errorf("storetest.MemoryProcessStore: %w", err)
	}
	if len(snapshots) == 0 {
		return nil
	}

	candidates := make([]core.ProcessSnapshot, len(snapshots))
	seen := make(map[string]struct{}, len(snapshots))
	for index, snapshot := range snapshots {
		if snapshot.Revision == math.MaxUint64 {
			return fmt.Errorf("storetest.MemoryProcessStore: snapshot[%d]: %w: revision is exhausted", index, core.ErrInvalidSnapshot)
		}
		if err := snapshot.Validate(); err != nil {
			return fmt.Errorf("storetest.MemoryProcessStore: snapshot[%d]: %w", index, err)
		}
		if _, duplicate := seen[snapshot.ID]; duplicate {
			return fmt.Errorf("storetest.MemoryProcessStore: snapshot[%d]: %w: duplicate process ID %q", index, core.ErrInvalidSnapshot, snapshot.ID)
		}
		seen[snapshot.ID] = struct{}{}
		candidate, err := cloneJSON(snapshot)
		if err != nil {
			return fmt.Errorf("storetest.MemoryProcessStore: snapshot[%d]: clone: %w", index, err)
		}
		candidates[index] = candidate
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return fmt.Errorf("storetest.MemoryProcessStore: %w", err)
	}
	for _, snapshot := range snapshots {
		actualRevision := uint64(0)
		if current, ok := s.snapshots[snapshot.ID]; ok {
			actualRevision = current.Revision
		}
		if actualRevision != snapshot.Revision {
			return &core.RevisionConflictError{
				ProcessID: snapshot.ID,
				Expected:  snapshot.Revision,
				Actual:    actualRevision,
			}
		}
	}
	for index, snapshot := range snapshots {
		candidates[index].Revision++
		s.snapshots[snapshot.ID] = candidates[index]
	}
	return nil
}

// Load implements [core.SnapshotReader].
func (s *MemoryProcessStore) Load(_ context.Context, id string) (core.ProcessSnapshot, error) {
	if s == nil {
		return core.ProcessSnapshot{}, errors.New("storetest.MemoryProcessStore: nil receiver")
	}
	s.mu.RLock()
	snapshot, ok := s.snapshots[id]
	s.mu.RUnlock()
	if !ok {
		return core.ProcessSnapshot{}, fmt.Errorf("storetest.MemoryProcessStore: load %q: %w", id, core.ErrSnapshotNotFound)
	}
	cloned, err := cloneJSON(snapshot)
	if err != nil {
		return core.ProcessSnapshot{}, fmt.Errorf("storetest.MemoryProcessStore: clone loaded snapshot: %w", err)
	}
	if cloned.Revision == 0 {
		return core.ProcessSnapshot{}, fmt.Errorf("storetest.MemoryProcessStore: %w: stored revision is zero", core.ErrInvalidSnapshot)
	}
	return cloned, nil
}

// Delete implements [core.SnapshotDeleter].
func (s *MemoryProcessStore) Delete(_ context.Context, id string) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	delete(s.snapshots, id)
	s.mu.Unlock()
	return nil
}

// List implements [core.SnapshotLister].
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

// MemorySessionStore is an in-memory test double for [core.SessionStore].
type MemorySessionStore struct {
	store *memoryStore[core.Session]
}

// NewMemorySessionStore returns an empty session-store test double.
func NewMemorySessionStore() *MemorySessionStore {
	return &MemorySessionStore{
		store: newMemoryStore[core.Session]("storetest.MemorySessionStore", core.ErrSessionNotFound),
	}
}

// Save implements [core.SessionWriter].
func (s *MemorySessionStore) Save(_ context.Context, session core.Session) error {
	if s == nil {
		return errors.New("storetest.MemorySessionStore: nil receiver")
	}
	snapshot, err := cloneSession(session)
	if err != nil {
		return fmt.Errorf("storetest.MemorySessionStore: snapshot %q: %w", session.ID, err)
	}
	return s.store.save(snapshot.ID, snapshot)
}

// Load implements [core.SessionReader].
func (s *MemorySessionStore) Load(_ context.Context, id string) (core.Session, error) {
	if s == nil {
		return core.Session{}, errors.New("storetest.MemorySessionStore: nil receiver")
	}
	stored, err := s.store.load(id)
	if err != nil {
		return core.Session{}, err
	}
	snapshot, err := cloneSession(stored)
	if err != nil {
		return core.Session{}, fmt.Errorf("storetest.MemorySessionStore: clone loaded session %q: %w", id, err)
	}
	return snapshot, nil
}

// Delete implements [core.SessionDeleter].
func (s *MemorySessionStore) Delete(_ context.Context, id string) error {
	if s == nil {
		return nil
	}
	s.store.delete(id)
	return nil
}

// List implements [core.SessionLister].
func (s *MemorySessionStore) List(_ context.Context) ([]string, error) {
	if s == nil {
		return nil, nil
	}
	ids := s.store.list()
	slices.Sort(ids)
	return ids, nil
}

type memoryStore[V any] struct {
	label    string
	notFound error

	mu    sync.RWMutex
	items map[string]V
}

func newMemoryStore[V any](label string, notFound error) *memoryStore[V] {
	return &memoryStore[V]{
		label:    label,
		notFound: notFound,
		items:    make(map[string]V),
	}
}

func (s *memoryStore[V]) save(id string, value V) error {
	if id == "" {
		return fmt.Errorf("%s: ID must not be empty", s.label)
	}
	s.mu.Lock()
	s.items[id] = value
	s.mu.Unlock()
	return nil
}

func (s *memoryStore[V]) load(id string) (V, error) {
	s.mu.RLock()
	value, ok := s.items[id]
	s.mu.RUnlock()
	if !ok {
		var zero V
		return zero, fmt.Errorf("%s: %w (ID=%q)", s.label, s.notFound, id)
	}
	return value, nil
}

func (s *memoryStore[V]) delete(id string) {
	s.mu.Lock()
	delete(s.items, id)
	s.mu.Unlock()
}

func (s *memoryStore[V]) list() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.items))
	for id := range s.items {
		ids = append(ids, id)
	}
	return ids
}

func cloneSession(session core.Session) (core.Session, error) {
	if err := session.Validate(); err != nil {
		return core.Session{}, err
	}
	return cloneJSON(session)
}

func cloneJSON[T any](value T) (T, error) {
	var clone T
	data, err := json.Marshal(value)
	if err != nil {
		return clone, err
	}
	if err := json.Unmarshal(data, &clone); err != nil {
		return clone, err
	}
	return clone, nil
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
