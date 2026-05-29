// Package persistence ships reference [core.ProcessStore] backends beyond
// the in-memory one in core. FileStore is a dependency-free, durable store
// that survives process restarts — suitable for single-node deployments,
// audit trails, and handing a paused process to a later run. Multi-node
// backends (postgres, redis, ...) belong in a dedicated module.
package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Tangerg/lynx/agent/core"
)

// FileStore persists each [core.ProcessSnapshot] as one JSON file in a
// directory, named by the URL-escaped process id (so arbitrary ids —
// including composite child ids with spaces or slashes — map to safe,
// reversible filenames). Writes are atomic (temp file + rename) so a crash
// mid-save never leaves a half-written snapshot.
//
// Round-trip note: [core.ProcessSnapshot.LastWorld] is an interface value
// that does not survive a JSON decode, so FileStore drops it on save. This
// is safe — the planner re-derives the world state on restore (the field is
// documented as informational). All other fields round-trip as generic
// JSON, subject to the same lossy-value constraints [core.ProcessSnapshot]
// already documents.
type FileStore struct {
	dir string
	mu  sync.Mutex // serializes the rename step so concurrent saves of the same id don't interleave
}

var _ core.ProcessStore = (*FileStore)(nil)

// NewFileStore creates a FileStore rooted at dir, creating the directory
// (and parents) if needed.
func NewFileStore(dir string) (*FileStore, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("persistence.NewFileStore: dir must not be empty")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("persistence.NewFileStore: create dir: %w", err)
	}
	return &FileStore{dir: dir}, nil
}

const fileSuffix = ".json"

// path maps a process id to its on-disk file. url.PathEscape keeps the name
// filesystem-safe and reversible.
func (s *FileStore) path(id string) string {
	return filepath.Join(s.dir, url.PathEscape(id)+fileSuffix)
}

// Save writes the snapshot atomically. LastWorld is cleared first (see type
// doc) so the JSON decodes cleanly on Load.
func (s *FileStore) Save(ctx context.Context, snapshot core.ProcessSnapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if snapshot.ID == "" {
		return errors.New("persistence.FileStore.Save: snapshot.ID must not be empty")
	}

	snapshot.LastWorld = nil

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("persistence.FileStore.Save: marshal %q: %w", snapshot.ID, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	final := s.path(snapshot.ID)
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("persistence.FileStore.Save: write %q: %w", snapshot.ID, err)
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("persistence.FileStore.Save: commit %q: %w", snapshot.ID, err)
	}
	return nil
}

// Load reads the snapshot for id, wrapping [core.ErrSnapshotNotFound] when
// no file exists.
func (s *FileStore) Load(ctx context.Context, id string) (core.ProcessSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return core.ProcessSnapshot{}, err
	}

	data, err := os.ReadFile(s.path(id))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return core.ProcessSnapshot{}, fmt.Errorf("persistence.FileStore.Load %q: %w", id, core.ErrSnapshotNotFound)
		}
		return core.ProcessSnapshot{}, fmt.Errorf("persistence.FileStore.Load: read %q: %w", id, err)
	}

	var snap core.ProcessSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return core.ProcessSnapshot{}, fmt.Errorf("persistence.FileStore.Load: decode %q: %w", id, err)
	}
	return snap, nil
}

// Delete removes the snapshot for id. Idempotent — an unknown id is not an
// error.
func (s *FileStore) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Remove(s.path(id)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("persistence.FileStore.Delete %q: %w", id, err)
	}
	return nil
}

// List returns every stored process id, recovered from the escaped
// filenames.
func (s *FileStore) List(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("persistence.FileStore.List: %w", err)
	}

	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, fileSuffix) {
			continue
		}
		id, err := url.PathUnescape(strings.TrimSuffix(name, fileSuffix))
		if err != nil {
			continue // not one of ours; skip
		}
		ids = append(ids, id)
	}
	return ids, nil
}
