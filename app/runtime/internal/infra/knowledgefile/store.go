// Package knowledgefile persists the user- and project-scoped LYRA.md
// documents. It owns filesystem layout and atomic replacement, but no knowledge
// policy or application workflow.
package knowledgefile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
//   - <cwd>/LYRA.md — workspace-local knowledge
//   - <project-root>/LYRA.md — project knowledge when the workspace is nested
//   - <data-dir>/LYRA.md — user scope (cross-project preferences)
//
// Files are created lazily on first Update. Every read returns an opaque content
// revision, and writes compare that revision while holding the store lock so two
// clients cannot silently overwrite each other's edits. Machine-curated facts
// never enter these human-owned files.
type Store struct {
	defaultWorkspaceDirectory string
	userScopeDirectory        string

	mu sync.Mutex // protects the file writes (paths differ but a single mutex is plenty for this volume)
}

// New binds both filesystem roots explicitly and only maps
// knowledge scopes onto the paths supplied at construction.
func New(userScopeDirectory, defaultWorkspaceDirectory string) (*Store, error) {
	if userScopeDirectory == "" {
		return nil, errors.New("knowledge store: user scope directory is required")
	}
	if !filepath.IsAbs(userScopeDirectory) {
		return nil, errors.New("knowledge store: user scope directory must be absolute")
	}
	if defaultWorkspaceDirectory == "" {
		return nil, errors.New("knowledge store: default workspace directory is required")
	}
	if !filepath.IsAbs(defaultWorkspaceDirectory) {
		return nil, errors.New("knowledge store: default workspace directory must be absolute")
	}
	return &Store{
		defaultWorkspaceDirectory: defaultWorkspaceDirectory,
		userScopeDirectory:        userScopeDirectory,
	}, nil
}

// pathFor maps a (scope, dir) pair to its absolute filesystem path.
// Empty dir falls back to the construction-time default. Returns an
// empty path when the project scope has neither a dir nor a resolvable default,
// while an unknown scope is rejected rather than reinterpreted as unavailable.
func (s *Store) pathFor(scope knowledge.Scope, dir string) (string, error) {
	switch scope {
	case knowledge.ScopeCWD, knowledge.ScopeProjectRoot:
		if dir == "" {
			dir = s.defaultWorkspaceDirectory
		}
		if !filepath.IsAbs(dir) {
			return "", errors.New("project directory must be absolute")
		}
		return filepath.Join(dir, knowledgeFileName), nil
	case knowledge.ScopeHome:
		return filepath.Join(s.userScopeDirectory, knowledgeFileName), nil
	default:
		return "", scope.Validate()
	}
}

func (s *Store) Get(_ context.Context, scope knowledge.Scope, dir string) (knowledge.Entry, error) {
	path, err := s.pathFor(scope, dir)
	if err != nil {
		return knowledge.Entry{}, fmt.Errorf("knowledge store: resolve path: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return readEntry(scope, path)
}

func readEntry(scope knowledge.Scope, path string) (knowledge.Entry, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return knowledge.Entry{Scope: scope, Path: path, Revision: contentRevision(nil)}, nil
	}
	if err != nil {
		return knowledge.Entry{}, fmt.Errorf("knowledge store: read %q: %w", path, err)
	}
	entry := knowledge.Entry{
		Scope: scope, Path: path, Content: string(data), Revision: contentRevision(data),
	}
	if info, err := os.Stat(path); err == nil {
		entry.UpdatedAt = info.ModTime()
	}
	return entry, nil
}

func (s *Store) Update(_ context.Context, scope knowledge.Scope, dir, expectedRevision, content string) (knowledge.Entry, error) {
	path, err := s.pathFor(scope, dir)
	if err != nil {
		return knowledge.Entry{}, fmt.Errorf("knowledge store: resolve path: %w", err)
	}
	if expectedRevision == "" {
		return knowledge.Entry{}, knowledge.ErrRevisionRequired
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := readEntry(scope, path)
	if err != nil {
		return knowledge.Entry{}, err
	}
	if current.Revision != expectedRevision {
		return knowledge.Entry{}, knowledge.ErrRevisionConflict
	}

	// Ensure the parent directory exists. The persistence bundle creates the
	// process data directory eagerly; a project directory can be supplied later
	// by a Session and therefore remains a per-write responsibility here.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return knowledge.Entry{}, fmt.Errorf("knowledge store: mkdir: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "."+knowledgeFileName+"-*")
	if err != nil {
		return knowledge.Entry{}, fmt.Errorf("knowledge store: create temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return knowledge.Entry{}, fmt.Errorf("knowledge store: set temporary file mode: %w", err)
	}
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return knowledge.Entry{}, fmt.Errorf("knowledge store: write temporary file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return knowledge.Entry{}, fmt.Errorf("knowledge store: sync temporary file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return knowledge.Entry{}, fmt.Errorf("knowledge store: close temporary file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return knowledge.Entry{}, fmt.Errorf("knowledge store: rename: %w", err)
	}
	// The replacement is already committed. Build the authoritative response
	// from the exact bytes we wrote so a transient post-rename read failure can
	// never turn a successful mutation into an apparent failure that a client
	// might retry with an obsolete revision.
	entry := knowledge.Entry{
		Scope: scope, Path: path, Content: content, Revision: contentRevision([]byte(content)),
	}
	if info, err := os.Stat(path); err == nil {
		entry.UpdatedAt = info.ModTime()
	}
	return entry, nil
}

func contentRevision(content []byte) string {
	digest := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(digest[:])
}

// List returns the visible cascade in precedence order: home, distinct project
// root, then cwd. A workspace at its project root has one physical file, so it
// is emitted once as cwd rather than duplicated under two scopes. Empty documents
// remain addressable and carry the revision a conditional create must use.
func (s *Store) List(ctx context.Context, cwd, projectRoot string) ([]knowledge.Entry, error) {
	type target struct {
		scope knowledge.Scope
		dir   string
	}
	targets := []target{{scope: knowledge.ScopeHome}}
	cwdPath, err := s.pathFor(knowledge.ScopeCWD, cwd)
	if err != nil {
		return nil, fmt.Errorf("knowledge store: resolve cwd path: %w", err)
	}
	projectPath, err := s.pathFor(knowledge.ScopeProjectRoot, projectRoot)
	if err != nil {
		return nil, fmt.Errorf("knowledge store: resolve project-root path: %w", err)
	}
	if projectPath != cwdPath {
		targets = append(targets, target{scope: knowledge.ScopeProjectRoot, dir: projectRoot})
	}
	targets = append(targets, target{scope: knowledge.ScopeCWD, dir: cwd})

	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]knowledge.Entry, 0, len(targets))
	for _, target := range targets {
		path, err := s.pathFor(target.scope, target.dir)
		if err != nil {
			return nil, fmt.Errorf("knowledge store: resolve listed path: %w", err)
		}
		entry, err := readEntry(target.scope, path)
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, nil
}
