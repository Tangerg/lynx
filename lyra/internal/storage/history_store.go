package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Tangerg/lynx/lyra/internal/service/history"
)

// FileHistoryStore persists the durable Item history one JSON file per
// session under <home>/history/<sessionId>.json. It implements
// [history.Store] so a session's items.list survives a backend restart
// with the same ids the live stream emitted.
//
// Each file holds the session's full {items, runs}; mutations rewrite it
// whole (atomic tmp+rename), guarded by a per-session lock. Item volume
// per session is modest (a handful per turn), so the whole-file rewrite
// trade-off matches FileInterruptStore / FileSessionService rather than
// the append-only FileMessageStore.
type FileHistoryStore struct {
	dir   string
	locks sync.Map // sessionID → *sync.Mutex
}

var _ history.Store = (*FileHistoryStore)(nil)

// sessionHistory is the on-disk shape of one session's history file.
type sessionHistory struct {
	Items []history.Item `json:"items"`
	Runs  []history.Run  `json:"runs"`
}

// NewFileHistoryStore prepares the <home>/history directory.
func NewFileHistoryStore() (*FileHistoryStore, error) {
	dir, err := SubDir("history")
	if err != nil {
		return nil, err
	}
	return &FileHistoryStore{dir: dir}, nil
}

func (s *FileHistoryStore) lockFor(sessionID string) *sync.Mutex {
	l, _ := s.locks.LoadOrStore(sessionID, &sync.Mutex{})
	return l.(*sync.Mutex)
}

func (s *FileHistoryStore) path(sessionID string) string {
	return filepath.Join(s.dir, sessionID+".json")
}

// AppendItem records one completed Item.
func (s *FileHistoryStore) AppendItem(_ context.Context, it history.Item) error {
	if it.SessionID == "" {
		return errors.New("storage: history AppendItem: empty sessionID")
	}
	mu := s.lockFor(it.SessionID)
	mu.Lock()
	defer mu.Unlock()

	h, err := s.read(it.SessionID)
	if err != nil {
		return err
	}
	h.Items = append(h.Items, it)
	return s.write(it.SessionID, h)
}

// PutRun records or replaces a RunRef keyed by RunID.
func (s *FileHistoryStore) PutRun(_ context.Context, r history.Run) error {
	if r.SessionID == "" {
		return errors.New("storage: history PutRun: empty sessionID")
	}
	mu := s.lockFor(r.SessionID)
	mu.Lock()
	defer mu.Unlock()

	h, err := s.read(r.SessionID)
	if err != nil {
		return err
	}
	replaced := false
	for i := range h.Runs {
		if h.Runs[i].RunID == r.RunID {
			h.Runs[i] = r
			replaced = true
			break
		}
	}
	if !replaced {
		h.Runs = append(h.Runs, r)
	}
	return s.write(r.SessionID, h)
}

// List returns the session's items (append order) + runs.
func (s *FileHistoryStore) List(_ context.Context, sessionID string) ([]history.Item, []history.Run, error) {
	mu := s.lockFor(sessionID)
	mu.Lock()
	defer mu.Unlock()

	h, err := s.read(sessionID)
	if err != nil {
		return nil, nil, err
	}
	return h.Items, h.Runs, nil
}

// read loads the session file; a missing file is an empty history.
func (s *FileHistoryStore) read(sessionID string) (sessionHistory, error) {
	var h sessionHistory
	data, err := os.ReadFile(s.path(sessionID))
	if errors.Is(err, os.ErrNotExist) || len(data) == 0 {
		return h, nil
	}
	if err != nil {
		return h, fmt.Errorf("storage: read history %q: %w", sessionID, err)
	}
	if err := json.Unmarshal(data, &h); err != nil {
		return h, fmt.Errorf("storage: decode history %q: %w", sessionID, err)
	}
	return h, nil
}

// write atomically replaces the session file (tmp + rename).
func (s *FileHistoryStore) write(sessionID string, h sessionHistory) error {
	data, err := json.Marshal(h)
	if err != nil {
		return fmt.Errorf("storage: encode history %q: %w", sessionID, err)
	}
	path := s.path(sessionID)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("storage: write history %q: %w", sessionID, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("storage: commit history %q: %w", sessionID, err)
	}
	return nil
}
