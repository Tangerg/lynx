package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Tangerg/lynx/lyra/internal/service/memory"
)

// memoryFileName is the on-disk file name for both scopes.
// "LYRA.md" on disk; rendered through MemoryService as a markdown
// blob the agent reads as project / user knowledge.
const memoryFileName = "LYRA.md"

// FileMemoryService persists [memory.Service] state to markdown
// files:
//
//   - <dir>/LYRA.md    — project scope (per-repo knowledge); dir is
//     supplied per call (a session's cwd), so one service serves
//     every project
//   - <home>/LYRA.md   — user scope    (cross-project preferences)
//
// Files are created lazily on first Update; Get returns "" until
// that point. Concurrent writes are serialized so `lyra memory edit`
// racing with the agent's auto-extract doesn't truncate either side.
type FileMemoryService struct {
	defaultDir string // fallback project dir for calls without one; empty if unavailable
	home       string // resolved from storage.Home()

	mu sync.Mutex // protects the file writes (paths differ but a single mutex is plenty for this volume)
}

// Compile-time tripwire: NewFileMemoryService returns the concrete type,
// so nothing checks memory.Service conformance until this assertion.
var _ memory.Service = (*FileMemoryService)(nil)

// NewFileMemoryService captures the process working directory (the
// per-call fallback project dir) and the storage home. Callers with a
// session in hand pass that session's cwd per call instead.
func NewFileMemoryService() (*FileMemoryService, error) {
	cwd, err := os.Getwd()
	if err != nil {
		// Non-fatal: the default project scope simply stays unavailable.
		cwd = ""
	}
	home, err := Home()
	if err != nil {
		return nil, fmt.Errorf("memory store: %w", err)
	}
	return &FileMemoryService{defaultDir: cwd, home: home}, nil
}

// pathFor maps a (scope, dir) pair to its absolute filesystem path.
// Empty dir falls back to the construction-time default. Returns an
// empty string when the scope is unavailable (project scope with
// neither a dir nor a resolvable default) so callers can skip cleanly.
func (s *FileMemoryService) pathFor(scope memory.Scope, dir string) string {
	switch scope {
	case memory.ScopeProject:
		if dir == "" {
			dir = s.defaultDir
		}
		if dir == "" {
			return ""
		}
		return filepath.Join(dir, memoryFileName)
	case memory.ScopeUser:
		return filepath.Join(s.home, memoryFileName)
	}
	return ""
}

// ------------------------------------------------------------------
// memory.Service
// ------------------------------------------------------------------

func (s *FileMemoryService) Get(_ context.Context, scope memory.Scope, dir string) (string, error) {
	path := s.pathFor(scope, dir)
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("memory store: read %q: %w", path, err)
	}
	return string(data), nil
}

func (s *FileMemoryService) Update(_ context.Context, scope memory.Scope, dir string, content string) error {
	path := s.pathFor(scope, dir)
	if path == "" {
		return fmt.Errorf("memory store: scope %d unavailable", scope)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	// Ensure the parent dir exists (user scope's parent is LYRA_HOME
	// which Home() already created; project scope's cwd we assume
	// the user has).
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("memory store: mkdir: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return fmt.Errorf("memory store: write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("memory store: rename: %w", err)
	}
	return nil
}

// List returns one [memory.Entry] per scope that has content. Empty
// scopes are skipped — the UI shouldn't render placeholder entries
// for files that don't exist yet.
func (s *FileMemoryService) List(ctx context.Context, dir string) ([]memory.Entry, error) {
	out := make([]memory.Entry, 0, 2)
	for _, scope := range []memory.Scope{memory.ScopeProject, memory.ScopeUser} {
		content, err := s.Get(ctx, scope, dir)
		if err != nil {
			return nil, err
		}
		if content == "" {
			continue
		}
		entry := memory.Entry{Scope: scope, Content: content}
		// CapturedAt = the LYRA.md file's mtime: it's a user-editable file, so
		// its last-modified time is the truthful "when this memory landed".
		// Best-effort — a stat failure leaves the zero time rather than
		// dropping the entry.
		if info, err := os.Stat(s.pathFor(scope, dir)); err == nil {
			entry.CapturedAt = info.ModTime()
		}
		out = append(out, entry)
	}
	return out, nil
}
