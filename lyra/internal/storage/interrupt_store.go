package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Tangerg/lynx/lyra/internal/service/interrupts"
)

// FileInterruptStore persists open HITL interrupts to one JSON file under
// the storage home. It implements [interrupts.Store] so the durable side
// of cross-restart resume — the interrupt metadata that runs.resume looks
// up by parentRunId — survives a backend restart alongside the agent
// process snapshot (see storage.FileProcessStore).
//
// Open interrupts are few (one per parked run) and read/written whole, so
// a single mutex-guarded map mirrored to one JSON file (atomic tmp+rename
// on every mutate) is the right shape — same trade-off as
// FileSessionService, not the append-only FileMessageStore.
type FileInterruptStore struct {
	mu      sync.Mutex
	pending map[string]interrupts.Pending // parentRunID → entry
	path    string
}

var _ interrupts.Store = (*FileInterruptStore)(nil)

// NewFileInterruptStore opens (or creates) the interrupts file under the
// storage home and loads any persisted entries.
func NewFileInterruptStore() (*FileInterruptStore, error) {
	dir, err := Home()
	if err != nil {
		return nil, err
	}
	s := &FileInterruptStore{
		pending: map[string]interrupts.Pending{},
		path:    filepath.Join(dir, "interrupts.json"),
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// load reads the on-disk file into the map. Missing file → empty store.
func (s *FileInterruptStore) load() error {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("storage: read %q: %w", s.path, err)
	}
	if len(data) == 0 {
		return nil
	}
	var list []interrupts.Pending
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("storage: parse %q: %w", s.path, err)
	}
	for _, p := range list {
		s.pending[p.ParentRunID] = p
	}
	return nil
}

// persist writes the current map to disk atomically. Caller holds s.mu.
func (s *FileInterruptStore) persist() error {
	list := make([]interrupts.Pending, 0, len(s.pending))
	for _, p := range s.pending {
		list = append(list, p)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("storage: marshal: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("storage: write tmp: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("storage: rename: %w", err)
	}
	return nil
}

// Put records (or replaces) an entry keyed by ParentRunID, then persists.
// On persist failure the in-memory map is rolled back so it stays
// consistent with disk.
func (s *FileInterruptStore) Put(_ context.Context, p interrupts.Pending) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, had := s.pending[p.ParentRunID]
	s.pending[p.ParentRunID] = p
	if err := s.persist(); err != nil {
		if had {
			s.pending[p.ParentRunID] = prev
		} else {
			delete(s.pending, p.ParentRunID)
		}
		return err
	}
	return nil
}

func (s *FileInterruptStore) List(_ context.Context, sessionID string) ([]interrupts.Pending, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]interrupts.Pending, 0, len(s.pending))
	for _, p := range s.pending {
		if sessionID != "" && p.SessionID != sessionID {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

func (s *FileInterruptStore) Get(_ context.Context, parentRunID string) (interrupts.Pending, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.pending[parentRunID]
	return p, ok, nil
}

// Delete removes the entry for parentRunID and persists. Absent entries
// are a no-op (no disk write).
func (s *FileInterruptStore) Delete(_ context.Context, parentRunID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, had := s.pending[parentRunID]
	if !had {
		return nil
	}
	delete(s.pending, parentRunID)
	if err := s.persist(); err != nil {
		s.pending[parentRunID] = prev
		return err
	}
	return nil
}
