package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/lyra/internal/service/session"
)

// FileSessionService persists [session.Service] state to one JSON
// file on disk — small enough to rewrite atomically on every
// mutation, large enough to survive process restart. Sessions are
// keyed by id; the on-disk shape is a flat array of [session.Session]
// values, which loads back into the in-memory map at construction.
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
	path string

	mu       sync.RWMutex
	sessions map[string]*session.Session
}

// NewFileSessionService opens (or creates) the sessions file under
// the storage home directory. Returns an error when the directory
// cannot be created or the existing file is unreadable / malformed.
func NewFileSessionService() (*FileSessionService, error) {
	dir, err := Home()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "sessions.json")

	svc := &FileSessionService{path: path, sessions: map[string]*session.Session{}}
	if err := svc.load(); err != nil {
		return nil, err
	}
	return svc, nil
}

// load reads the on-disk file into the in-memory map. Missing file
// is treated as an empty store — first-run starts clean.
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
	for i := range list {
		sess := list[i]
		s.sessions[sess.ID] = &sess
	}
	return nil
}

// persist writes the current map to disk atomically — write to a
// tmp file, then rename. Called under the write lock so the
// serialized snapshot matches the in-memory state.
func (s *FileSessionService) persist() error {
	list := make([]session.Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		list = append(list, *sess)
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

// ------------------------------------------------------------------
// session.Service
// ------------------------------------------------------------------

func (s *FileSessionService) List(_ context.Context) ([]session.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]session.Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		out = append(out, *sess)
	}
	return out, nil
}

func (s *FileSessionService) Get(_ context.Context, id string) (session.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	if !ok {
		return session.Session{}, session.ErrNotFound
	}
	return *sess, nil
}

func (s *FileSessionService) Create(_ context.Context, title string) (session.Session, error) {
	now := time.Now().UTC()
	sess := &session.Session{
		ID:        uuid.NewString(),
		Title:     title,
		StartedAt: now,
		UpdatedAt: now,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sess.ID] = sess
	if err := s.persist(); err != nil {
		delete(s.sessions, sess.ID)
		return session.Session{}, err
	}
	return *sess, nil
}

func (s *FileSessionService) Fork(_ context.Context, parentID, atMessageID string) (session.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	parent, ok := s.sessions[parentID]
	if !ok {
		return session.Session{}, session.ErrNotFound
	}
	now := time.Now().UTC()
	child := &session.Session{
		ID:        uuid.NewString(),
		Title:     parent.Title + " (fork)",
		ParentID:  parentID,
		StartedAt: now,
		UpdatedAt: now,
		Metadata: map[string]string{
			"fork_at_message_id": atMessageID,
		},
	}
	s.sessions[child.ID] = child
	if err := s.persist(); err != nil {
		delete(s.sessions, child.ID)
		return session.Session{}, err
	}
	return *child, nil
}

func (s *FileSessionService) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.sessions[id]; !exists {
		return nil
	}
	saved := s.sessions[id]
	delete(s.sessions, id)
	if err := s.persist(); err != nil {
		s.sessions[id] = saved
		return err
	}
	return nil
}

// Touch updates UpdatedAt + bumps TurnCount and persists. Internal
// — same shape as [session.NewInMemoryService]'s Touch (mirrored
// here so callers can swap implementations without losing the
// recency bump).
func (s *FileSessionService) Touch(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return session.ErrNotFound
	}
	prev := *sess
	sess.UpdatedAt = time.Now().UTC()
	sess.TurnCount++
	if err := s.persist(); err != nil {
		*sess = prev
		return err
	}
	return nil
}
