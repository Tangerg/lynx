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

// FileMemoryService persists [memory.Service] state to two
// markdown files:
//
//   - <cwd>/LYRA.md    — project scope (per-repo knowledge)
//   - <home>/LYRA.md   — user scope    (cross-project preferences)
//
// Files are created lazily on first Update; Get returns "" until
// that point. Concurrent writes are serialized per-scope so
// `lyra memory edit` racing with the agent's auto-extract doesn't
// truncate either side.
type FileMemoryService struct {
	cwd  string // resolved at construction time; empty if unavailable
	home string // resolved from storage.Home()

	mu sync.Mutex // protects the per-scope file writes (project + user are separate paths but a single mutex is plenty for this volume)
}

// NewFileMemoryService captures the current working directory and
// the storage home. cwd is used as-is; if the agent later
// switches directories the project scope still points at the
// original cwd (Lyra is a per-invocation tool, not a daemon).
func NewFileMemoryService() (*FileMemoryService, error) {
	cwd, err := os.Getwd()
	if err != nil {
		// Non-fatal: project scope simply stays unavailable.
		cwd = ""
	}
	home, err := Home()
	if err != nil {
		return nil, fmt.Errorf("memory store: %w", err)
	}
	return &FileMemoryService{cwd: cwd, home: home}, nil
}

// pathFor maps a Scope to its absolute filesystem path. Returns an
// empty string when the scope is unavailable (project scope on a
// process that couldn't resolve cwd) so callers can skip cleanly.
func (s *FileMemoryService) pathFor(scope memory.Scope) string {
	switch scope {
	case memory.ScopeProject:
		if s.cwd == "" {
			return ""
		}
		return filepath.Join(s.cwd, memoryFileName)
	case memory.ScopeUser:
		return filepath.Join(s.home, memoryFileName)
	}
	return ""
}

// ------------------------------------------------------------------
// memory.Service
// ------------------------------------------------------------------

func (s *FileMemoryService) Get(_ context.Context, scope memory.Scope) (string, error) {
	path := s.pathFor(scope)
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

func (s *FileMemoryService) Update(_ context.Context, scope memory.Scope, content string) error {
	path := s.pathFor(scope)
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
func (s *FileMemoryService) List(ctx context.Context) ([]memory.Entry, error) {
	out := make([]memory.Entry, 0, 2)
	for _, scope := range []memory.Scope{memory.ScopeProject, memory.ScopeUser} {
		content, err := s.Get(ctx, scope)
		if err != nil {
			return nil, err
		}
		if content == "" {
			continue
		}
		out = append(out, memory.Entry{
			Scope:   scope,
			Content: content,
		})
	}
	return out, nil
}
