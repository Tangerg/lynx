package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Tangerg/lynx/lyra/internal/service/session"
)

// FileSessionService persists [session.Service] state to one JSON
// file on disk — small enough to rewrite atomically on every
// mutation, large enough to survive process restart. The
// in-memory shape (map + RWMutex + the mutate operations) is
// owned by [session.Repo]; this type composes Repo with a
// persist-on-mutate hook.
//
// Trade-offs:
//   - Whole-file rewrite per mutation: simple, atomic via rename;
//     fine while session count stays in the dozens. Switch to a
//     proper KV store once we have thousands.
//   - JSON not JSONL: makes the round-trip a single
//     marshal/unmarshal and lets `jq` browse the file casually.
//   - No fsync on write: durability is best-effort; the runtime
//     does not promise per-write durability today.
type FileSessionService struct {
	repo *session.Repo
	path string
}

// Compile-time tripwire: FileSessionService is handed to callers as a
// session.Service. NewFileSessionService returns the concrete type, so
// the interface conformance isn't checked by a constructor return —
// this assertion catches drift if either side's signature changes.
var _ session.Service = (*FileSessionService)(nil)

// NewFileSessionService opens (or creates) the sessions file
// under the storage home directory. Returns an error when the
// directory cannot be created or the existing file is unreadable
// / malformed.
func NewFileSessionService() (*FileSessionService, error) {
	dir, err := Home()
	if err != nil {
		return nil, err
	}
	svc := &FileSessionService{
		repo: session.NewRepo(),
		path: filepath.Join(dir, "sessions.json"),
	}
	if err := svc.load(); err != nil {
		return nil, err
	}
	return svc, nil
}

// load reads the on-disk file into the repo. Missing file is
// treated as an empty store — first-run starts clean.
func (s *FileSessionService) load() error {
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
	var list []session.Session
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("storage: parse %q: %w", s.path, err)
	}
	s.repo.Restore(list)
	return nil
}

// persist writes the current repo snapshot to disk atomically —
// tmp file + rename. Called after every successful mutate.
func (s *FileSessionService) persist() error {
	data, err := json.MarshalIndent(s.repo.List(), "", "  ")
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

// ------------------------------------------------------------------
// session.Service
// ------------------------------------------------------------------

// Read methods are straight pass-throughs to the repo — no I/O,
// nothing to roll back.

func (s *FileSessionService) List(_ context.Context) ([]session.Session, error) {
	return s.repo.List(), nil
}

func (s *FileSessionService) Get(_ context.Context, id string) (session.Session, error) {
	sess, ok := s.repo.Get(id)
	if !ok {
		return session.Session{}, session.ErrNotFound
	}
	return sess, nil
}

// Write methods all share the same shape: mutate the repo, try
// to persist, undo the repo change on persist failure so the
// in-memory state stays consistent with disk.

func (s *FileSessionService) Create(_ context.Context, title string) (session.Session, error) {
	sess := s.repo.Create(title)
	if err := s.persist(); err != nil {
		s.repo.Delete(sess.ID)
		return session.Session{}, err
	}
	return sess, nil
}

func (s *FileSessionService) Fork(_ context.Context, parentID, atMessageID string) (session.Session, error) {
	child, ok := s.repo.Fork(parentID, atMessageID)
	if !ok {
		return session.Session{}, session.ErrNotFound
	}
	if err := s.persist(); err != nil {
		s.repo.Delete(child.ID)
		return session.Session{}, err
	}
	return child, nil
}

func (s *FileSessionService) Delete(_ context.Context, id string) error {
	removed, found := s.repo.Delete(id)
	if !found {
		return nil // idempotent
	}
	if err := s.persist(); err != nil {
		s.repo.Insert(removed)
		return err
	}
	return nil
}

// Touch refreshes UpdatedAt + bumps TurnCount on the named
// session and persists. Mirrors session.inMemoryService.Touch —
// lives off the [session.Service] interface because it's
// implementation-detail bookkeeping.
//
// Rollback is best-effort: we capture the pre-mutation snapshot
// only when persist fails, restoring the previous fields.
func (s *FileSessionService) Touch(id string) error {
	prev, ok := s.repo.Get(id)
	if !ok {
		return session.ErrNotFound
	}
	if !s.repo.Touch(id) {
		return session.ErrNotFound
	}
	if err := s.persist(); err != nil {
		s.repo.Insert(prev)
		return err
	}
	return nil
}
