package storetest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/Tangerg/lynx/agent/core"
)

// MemoryProcessStore is a strict in-memory test double for [core.ProcessStore].
// It applies the JSON ownership boundary without prescribing how production
// adapters coordinate their storage.
type MemoryProcessStore struct {
	mu        sync.RWMutex
	snapshots map[string]core.ProcessSnapshot
}

var _ core.ProcessStore = (*MemoryProcessStore)(nil)

// NewMemoryProcessStore returns an empty process-store test double.
func NewMemoryProcessStore() *MemoryProcessStore {
	return &MemoryProcessStore{snapshots: make(map[string]core.ProcessSnapshot)}
}

// Save implements [core.ProcessStore].
func (s *MemoryProcessStore) Save(ctx context.Context, snapshots []core.ProcessSnapshot) error {
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
	for index, snapshot := range snapshots {
		candidate, err := cloneJSON(snapshot)
		if err != nil {
			return fmt.Errorf("storetest.MemoryProcessStore: snapshots[%d]: clone: %w", index, err)
		}
		candidates[index] = candidate
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return fmt.Errorf("storetest.MemoryProcessStore: %w", err)
	}
	for index, snapshot := range snapshots {
		s.snapshots[snapshot.ID] = candidates[index]
	}
	return nil
}

// Load implements [core.ProcessStore].
func (s *MemoryProcessStore) Load(ctx context.Context, id string) (core.ProcessSnapshot, error) {
	if s == nil {
		return core.ProcessSnapshot{}, errors.New("storetest.MemoryProcessStore: nil receiver")
	}
	if err := contextError(ctx); err != nil {
		return core.ProcessSnapshot{}, fmt.Errorf("storetest.MemoryProcessStore: %w", err)
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
	return cloned, nil
}

// List implements [core.ProcessStore].
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

// Delete implements [core.ProcessStore].
func (s *MemoryProcessStore) Delete(ctx context.Context, rootID string) error {
	if s == nil {
		return errors.New("storetest.MemoryProcessStore: nil receiver")
	}
	if err := contextError(ctx); err != nil {
		return fmt.Errorf("storetest.MemoryProcessStore: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	children := make(map[string][]string)
	for id, snapshot := range s.snapshots {
		if snapshot.ParentID != "" {
			children[snapshot.ParentID] = append(children[snapshot.ParentID], id)
		}
	}
	var remove func(string)
	remove = func(id string) {
		delete(s.snapshots, id)
		for _, childID := range children[id] {
			remove(childID)
		}
	}
	remove(rootID)
	return nil
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
