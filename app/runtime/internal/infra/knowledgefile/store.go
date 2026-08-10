// Package knowledgefile persists the user- and project-scoped LYRA.md
// documents. It owns filesystem layout and atomic replacement, but no knowledge
// policy or application workflow.
package knowledgefile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
)

// knowledgeFileName is the on-disk file name for both scopes.
// "LYRA.md" on disk; rendered through the knowledge store as a markdown
// blob consumed as project or user knowledge.
const knowledgeFileName = "LYRA.md"

// Store persists human-authored knowledge to markdown files:
//
//   - <dir>/LYRA.md    — project scope (per-repo knowledge); dir is
//     supplied per call (a session's cwd), so one store serves
//     every project
//   - <data-dir>/LYRA.md — user scope (cross-project preferences)
//
// Files are created lazily on first Update; Get returns "" until that point.
// Concurrent updates are serialized with last-write-wins semantics. Machine-
// curated facts never enter these human-owned files.
type Store struct {
	defaultProjectDirectory string
	userScopeDirectory      string

	mu sync.Mutex // protects the file writes (paths differ but a single mutex is plenty for this volume)
}

// New binds both filesystem roots explicitly and only maps
// knowledge scopes onto the paths supplied at construction.
func New(userScopeDirectory, defaultProjectDirectory string) (*Store, error) {
	if userScopeDirectory == "" {
		return nil, errors.New("knowledge store: user scope directory is required")
	}
	if !filepath.IsAbs(userScopeDirectory) {
		return nil, errors.New("knowledge store: user scope directory must be absolute")
	}
	if defaultProjectDirectory == "" {
		return nil, errors.New("knowledge store: default project directory is required")
	}
	if !filepath.IsAbs(defaultProjectDirectory) {
		return nil, errors.New("knowledge store: default project directory must be absolute")
	}
	return &Store{
		defaultProjectDirectory: defaultProjectDirectory,
		userScopeDirectory:      userScopeDirectory,
	}, nil
}

// pathFor maps a (scope, dir) pair to its absolute filesystem path.
// Empty dir falls back to the construction-time default. Returns an
// empty path when the project scope has neither a dir nor a resolvable default,
// while an unknown scope is rejected rather than reinterpreted as unavailable.
func (s *Store) pathFor(scope knowledge.Scope, dir string) (string, error) {
	switch scope {
	case knowledge.ScopeProject:
		if dir == "" {
			dir = s.defaultProjectDirectory
		}
		if !filepath.IsAbs(dir) {
			return "", errors.New("project directory must be absolute")
		}
		return filepath.Join(dir, knowledgeFileName), nil
	case knowledge.ScopeUser:
		return filepath.Join(s.userScopeDirectory, knowledgeFileName), nil
	default:
		return "", scope.Validate()
	}
}

func (s *Store) Get(_ context.Context, scope knowledge.Scope, dir string) (string, error) {
	path, err := s.pathFor(scope, dir)
	if err != nil {
		return "", fmt.Errorf("knowledge store: resolve path: %w", err)
	}
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("knowledge store: read %q: %w", path, err)
	}
	return string(data), nil
}

func (s *Store) Update(_ context.Context, scope knowledge.Scope, dir string, content string) error {
	path, err := s.pathFor(scope, dir)
	if err != nil {
		return fmt.Errorf("knowledge store: resolve path: %w", err)
	}
	if path == "" {
		return errors.New("knowledge store: project scope unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	// Ensure the parent directory exists. The persistence bundle creates the
	// process data directory eagerly; a project directory can be supplied later
	// by a Session and therefore remains a per-write responsibility here.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("knowledge store: mkdir: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "."+knowledgeFileName+"-*")
	if err != nil {
		return fmt.Errorf("knowledge store: create temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("knowledge store: set temporary file mode: %w", err)
	}
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("knowledge store: write temporary file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("knowledge store: sync temporary file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("knowledge store: close temporary file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("knowledge store: rename: %w", err)
	}
	return nil
}

// List returns one [knowledge.Entry] per scope that has content. A missing
// document is absence, so empty scopes are omitted.
func (s *Store) List(ctx context.Context, dir string) ([]knowledge.Entry, error) {
	out := make([]knowledge.Entry, 0, 2)
	for _, scope := range []knowledge.Scope{knowledge.ScopeProject, knowledge.ScopeUser} {
		content, err := s.Get(ctx, scope, dir)
		if err != nil {
			return nil, err
		}
		if content == "" {
			continue
		}
		entry := knowledge.Entry{Scope: scope, Content: content}
		// UpdatedAt = the LYRA.md file's mtime: it's a user-editable file, so
		// its last-modified time is the truthful "when this knowledge was updated".
		// Best-effort — a stat failure leaves the zero time rather than
		// dropping the entry.
		path, err := s.pathFor(scope, dir)
		if err != nil {
			return nil, fmt.Errorf("knowledge store: resolve listed path: %w", err)
		}
		if info, err := os.Stat(path); err == nil {
			entry.UpdatedAt = info.ModTime()
		}
		out = append(out, entry)
	}
	return out, nil
}
