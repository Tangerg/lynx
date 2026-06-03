package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Tangerg/lynx/agent/core"
)

// FileProcessStore persists agent-process snapshots one JSON file per
// process under <home>/processes. It implements [core.ProcessStore] so
// the engine's auto-snapshot hook (engine.Config.ProcessStore) can park
// a suspended process to disk and a later runs.resume can rebuild it
// after a restart.
//
// One file per snapshot (keyed by [core.ProcessSnapshot.ID]) rather than
// a single rewritten map: snapshots are read/written whole, are sizable
// (full blackboard + history), and there is one per live/parked process,
// so per-key files keep each Save O(snapshot) instead of O(all). Writes
// are atomic via tmp + rename; per-process locks let distinct processes
// snapshot concurrently while the same id serializes.
type FileProcessStore struct {
	dir string

	// locks holds per-process mutexes (read-mostly: the pointer is
	// created once per id then only looked up). Mirrors FileMessageStore.
	locks sync.Map // map[processID]*sync.Mutex
}

var _ core.ProcessStore = (*FileProcessStore)(nil)

// NewFileProcessStore opens <home>/processes and returns a ready store.
// The directory is created lazily by [SubDir].
func NewFileProcessStore() (*FileProcessStore, error) {
	dir, err := SubDir("processes")
	if err != nil {
		return nil, err
	}
	return &FileProcessStore{dir: dir}, nil
}

// pathFor returns the JSON file for processID. Rejects ids containing
// path separators or "." / ".." so a stray id can't escape the dir.
func (s *FileProcessStore) pathFor(id string) (string, error) {
	if id == "" || id == "." || id == ".." {
		return "", fmt.Errorf("storage: invalid process id %q", id)
	}
	for _, r := range id {
		if r == '/' || r == '\\' || r == 0 {
			return "", fmt.Errorf("storage: invalid process id %q", id)
		}
	}
	return filepath.Join(s.dir, id+".json"), nil
}

// lockFor returns a per-process mutex, allocating one on first use.
func (s *FileProcessStore) lockFor(id string) *sync.Mutex {
	if existing, ok := s.locks.Load(id); ok {
		return existing.(*sync.Mutex)
	}
	actual, _ := s.locks.LoadOrStore(id, &sync.Mutex{})
	return actual.(*sync.Mutex)
}

// Save persists snapshot under its id, overwriting any existing entry.
func (s *FileProcessStore) Save(ctx context.Context, snapshot core.ProcessSnapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.pathFor(snapshot.ID)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("storage: marshal snapshot: %w", err)
	}
	lock := s.lockFor(snapshot.ID)
	lock.Lock()
	defer lock.Unlock()

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("storage: write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("storage: rename: %w", err)
	}
	return nil
}

// Load returns the snapshot for id, or an error wrapping
// [core.ErrSnapshotNotFound] when the id is unknown.
func (s *FileProcessStore) Load(ctx context.Context, id string) (core.ProcessSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return core.ProcessSnapshot{}, err
	}
	path, err := s.pathFor(id)
	if err != nil {
		return core.ProcessSnapshot{}, err
	}
	lock := s.lockFor(id)
	lock.Lock()
	defer lock.Unlock()

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return core.ProcessSnapshot{}, fmt.Errorf("storage: load %q: %w", id, core.ErrSnapshotNotFound)
	}
	if err != nil {
		return core.ProcessSnapshot{}, fmt.Errorf("storage: read %q: %w", path, err)
	}
	var snap core.ProcessSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return core.ProcessSnapshot{}, fmt.Errorf("storage: parse %q: %w", path, err)
	}
	return snap, nil
}

// Delete removes the snapshot for id. Idempotent — unknown id is not an
// error.
func (s *FileProcessStore) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.pathFor(id)
	if err != nil {
		return err
	}
	lock := s.lockFor(id)
	lock.Lock()
	defer lock.Unlock()

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("storage: remove %q: %w", path, err)
	}
	return nil
}

// List returns every stored process id (the .json files in the dir,
// suffix stripped). A missing dir yields an empty list.
func (s *FileProcessStore) List(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("storage: read dir %q: %w", s.dir, err)
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		out = append(out, strings.TrimSuffix(name, ".json"))
	}
	return out, nil
}
