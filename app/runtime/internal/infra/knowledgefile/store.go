// Package knowledgefile persists the user- and project-scoped LYRA.md
// documents. It owns filesystem layout and atomic replacement, but no knowledge
// policy or application workflow.
package knowledgefile

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/advisorylock"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/pathidentity"
)

// knowledgeFileName is the on-disk file name for both scopes.
// "LYRA.md" on disk; rendered through the knowledge store as a markdown
// blob consumed as project or user knowledge.
const knowledgeFileName = "LYRA.md"

const stagedFilePrefix = ".LYRA.md.lyra-stage-"

const stagedRecoveryReadBatchSize = 128

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
	recoveredDirectories      map[string]struct{}

	mu sync.Mutex // serializes this Store's recovery and compare-and-replace decisions
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
		recoveredDirectories:      make(map[string]struct{}),
	}, nil
}

// rootFor maps a semantic scope to the filesystem root that contains its one
// knowledge document. Physical identity and containment remain an Infra concern.
func (s *Store) rootFor(scope knowledge.Scope, dir string) (string, error) {
	switch scope {
	case knowledge.ScopeCWD, knowledge.ScopeProjectRoot:
		if dir == "" {
			dir = s.defaultWorkspaceDirectory
		}
		if !filepath.IsAbs(dir) {
			return "", errors.New("project directory must be absolute")
		}
		return filepath.Clean(dir), nil
	case knowledge.ScopeHome:
		return s.userScopeDirectory, nil
	default:
		return "", scope.Validate()
	}
}

type document struct {
	scope    knowledge.Scope
	root     string
	relative string
	path     string
}

func (s *Store) documentFor(scope knowledge.Scope, dir string) (document, error) {
	root, err := s.rootFor(scope, dir)
	if err != nil {
		return document{}, err
	}
	physicalRoot, err := pathidentity.Resolve("", root)
	if err != nil {
		return document{}, fmt.Errorf("resolve knowledge scope root %q: %w", root, err)
	}
	physicalPath, err := pathidentity.Resolve(physicalRoot, knowledgeFileName)
	if err != nil {
		return document{}, fmt.Errorf("resolve knowledge document in %q: %w", root, err)
	}
	inside, err := pathidentity.Contains(physicalRoot, physicalPath)
	if err != nil {
		return document{}, err
	}
	if !inside {
		return document{}, fmt.Errorf("%w: %q resolves outside %q", knowledge.ErrPathOutsideScope, filepath.Join(root, knowledgeFileName), root)
	}
	relative, err := filepath.Rel(physicalRoot, physicalPath)
	if err != nil {
		return document{}, fmt.Errorf("make knowledge path relative to scope: %w", err)
	}
	return document{scope: scope, root: physicalRoot, relative: relative, path: physicalPath}, nil
}

func (s *Store) Get(ctx context.Context, scope knowledge.Scope, dir string) (knowledge.Entry, error) {
	doc, err := s.documentFor(scope, dir)
	if err != nil {
		return knowledge.Entry{}, fmt.Errorf("knowledge store: resolve path: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, _, err := s.read(ctx, doc)
	return entry, err
}

func (s *Store) read(ctx context.Context, doc document) (knowledge.Entry, os.FileMode, error) {
	if err := s.recoverStagedFiles(ctx, doc, false); err != nil {
		return knowledge.Entry{}, 0, err
	}
	root, err := os.OpenRoot(doc.root)
	if errors.Is(err, os.ErrNotExist) {
		return emptyEntry(doc), initialMode(doc.scope), nil
	}
	if err != nil {
		return knowledge.Entry{}, 0, fmt.Errorf("knowledge store: open scope root %q: %w", doc.root, err)
	}
	defer func() { _ = root.Close() }()
	return readDocumentAt(ctx, root, doc)
}

func readDocumentAt(ctx context.Context, root *os.Root, doc document) (knowledge.Entry, os.FileMode, error) {
	if cause := context.Cause(ctx); cause != nil {
		return knowledge.Entry{}, 0, cause
	}
	file, err := root.Open(doc.relative)
	if errors.Is(err, os.ErrNotExist) {
		return emptyEntry(doc), initialMode(doc.scope), nil
	}
	if err != nil {
		return knowledge.Entry{}, 0, fmt.Errorf("knowledge store: open %q: %w", doc.path, err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return knowledge.Entry{}, 0, fmt.Errorf("knowledge store: inspect %q: %w", doc.path, err)
	}
	if !info.Mode().IsRegular() {
		return knowledge.Entry{}, 0, fmt.Errorf("knowledge store: %q is not a regular file", doc.path)
	}
	if err := knowledge.ValidateDocumentSize(info.Size()); err != nil {
		return knowledge.Entry{}, 0, fmt.Errorf("knowledge store: inspect %q: %w", doc.path, err)
	}
	data, err := io.ReadAll(io.LimitReader(
		knowledgeContextReader{ctx: ctx, reader: file},
		knowledge.MaxDocumentBytes+1,
	))
	if err != nil {
		return knowledge.Entry{}, 0, fmt.Errorf("knowledge store: read %q: %w", doc.path, err)
	}
	if err := knowledge.ValidateDocumentSize(int64(len(data))); err != nil {
		return knowledge.Entry{}, 0, fmt.Errorf("knowledge store: read %q: %w", doc.path, err)
	}
	entry := knowledge.Entry{
		Scope: doc.scope, Path: doc.path, Content: string(data), Revision: contentRevision(data),
		UpdatedAt: info.ModTime(),
	}
	return entry, info.Mode().Perm(), nil
}

func emptyEntry(doc document) knowledge.Entry {
	return knowledge.Entry{Scope: doc.scope, Path: doc.path, Revision: contentRevision(nil)}
}

func initialMode(scope knowledge.Scope) os.FileMode {
	if scope == knowledge.ScopeHome {
		return 0o600
	}
	return 0o644
}

func initialDirectoryMode(scope knowledge.Scope) os.FileMode {
	if scope == knowledge.ScopeHome {
		return 0o700
	}
	return 0o755
}

func (s *Store) Update(ctx context.Context, scope knowledge.Scope, dir, expectedRevision, content string) (knowledge.Entry, error) {
	if expectedRevision == "" {
		return knowledge.Entry{}, knowledge.ErrRevisionRequired
	}
	if err := knowledge.ValidateDocument(content); err != nil {
		return knowledge.Entry{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rootPath, err := s.rootFor(scope, dir)
	if err != nil {
		return knowledge.Entry{}, fmt.Errorf("knowledge store: resolve root: %w", err)
	}
	if err := os.MkdirAll(rootPath, initialDirectoryMode(scope)); err != nil {
		return knowledge.Entry{}, fmt.Errorf("knowledge store: mkdir: %w", err)
	}
	doc, err := s.documentFor(scope, dir)
	if err != nil {
		return knowledge.Entry{}, fmt.Errorf("knowledge store: resolve path: %w", err)
	}
	root, err := os.OpenRoot(doc.root)
	if err != nil {
		return knowledge.Entry{}, fmt.Errorf("knowledge store: open scope root %q: %w", doc.root, err)
	}
	defer func() { _ = root.Close() }()
	if err := root.MkdirAll(filepath.Dir(doc.relative), initialDirectoryMode(scope)); err != nil {
		return knowledge.Entry{}, fmt.Errorf("knowledge store: create document directory: %w", err)
	}
	lease, err := advisorylock.AcquireDirectory(ctx, filepath.Dir(doc.path))
	if err != nil {
		return knowledge.Entry{}, fmt.Errorf("knowledge store: acquire document lease: %w", err)
	}
	defer func() { _ = lease.Release() }()
	if err := s.recoverStagedFilesAt(ctx, root, doc, true); err != nil {
		return knowledge.Entry{}, err
	}
	current, mode, err := readDocumentAt(ctx, root, doc)
	if err != nil {
		return knowledge.Entry{}, err
	}
	if current.Revision != expectedRevision {
		return knowledge.Entry{}, knowledge.ErrRevisionConflict
	}

	tmp, tmpPath, err := createTemporary(root, doc.relative)
	if err != nil {
		return knowledge.Entry{}, fmt.Errorf("knowledge store: create temporary file: %w", err)
	}
	defer func() { _ = root.Remove(tmpPath) }()
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return knowledge.Entry{}, fmt.Errorf("knowledge store: write temporary file: %w", err)
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return knowledge.Entry{}, fmt.Errorf("knowledge store: set temporary file mode: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return knowledge.Entry{}, fmt.Errorf("knowledge store: sync temporary file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return knowledge.Entry{}, fmt.Errorf("knowledge store: close temporary file: %w", err)
	}
	if err := root.Rename(tmpPath, doc.relative); err != nil {
		return knowledge.Entry{}, fmt.Errorf("knowledge store: rename: %w", err)
	}
	committed := knowledge.Entry{
		Scope: doc.scope, Path: doc.path, Content: content, Revision: contentRevision([]byte(content)),
	}
	if info, err := root.Stat(doc.relative); err == nil {
		committed.UpdatedAt = info.ModTime()
	}
	// A directory sync strengthens crash durability on filesystems that support
	// it. The rename is already committed, so a platform-specific sync refusal
	// cannot be returned as a command failure without making settlement ambiguous.
	syncCommittedDirectory(root, filepath.Dir(doc.relative))
	// The replacement is already committed. Build the authoritative response
	// from the exact bytes we wrote so a transient post-rename read failure can
	// never turn a successful mutation into an apparent failure that a client
	// might retry with an obsolete revision.
	return committed, nil
}

type knowledgeContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (k knowledgeContextReader) Read(buffer []byte) (int, error) {
	if cause := context.Cause(k.ctx); cause != nil {
		return 0, cause
	}
	read, err := k.reader.Read(buffer)
	if cause := context.Cause(k.ctx); cause != nil {
		return read, cause
	}
	return read, err
}

func createTemporary(root *os.Root, target string) (*os.File, string, error) {
	directory := filepath.Dir(target)
	for range 10 {
		path := filepath.Join(directory, stagedFilePrefix+rand.Text())
		file, err := root.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return nil, "", err
		}
		return file, path, nil
	}
	return nil, "", errors.New("knowledge store: temporary file name collisions exhausted")
}

func removeStagedFiles(ctx context.Context, root *os.Root, target string) error {
	for {
		paths, err := stagedFilesForRemoval(ctx, root, target)
		if err != nil {
			return err
		}
		if len(paths) == 0 {
			return nil
		}
		for _, path := range paths {
			if cause := context.Cause(ctx); cause != nil {
				return cause
			}
			info, err := root.Lstat(path)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				continue
			}
			if err := root.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
}

func stagedFilesForRemoval(ctx context.Context, root *os.Root, target string) (paths []string, err error) {
	directoryPath := filepath.Dir(target)
	directory, err := root.Open(directoryPath)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, directory.Close()) }()

	for len(paths) < stagedRecoveryReadBatchSize {
		if cause := context.Cause(ctx); cause != nil {
			return nil, cause
		}
		entries, readErr := directory.ReadDir(stagedRecoveryReadBatchSize)
		for _, entry := range entries {
			if cause := context.Cause(ctx); cause != nil {
				return nil, cause
			}
			if !isStagedFileName(entry.Name()) {
				continue
			}
			path := filepath.Join(directoryPath, entry.Name())
			info, err := root.Lstat(path)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, err
			}
			if !info.Mode().IsRegular() {
				continue
			}
			paths = append(paths, path)
			if len(paths) == stagedRecoveryReadBatchSize {
				return paths, nil
			}
		}
		if errors.Is(readErr, io.EOF) {
			return paths, nil
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	return paths, nil
}

func (s *Store) recoverStagedFiles(ctx context.Context, doc document, force bool) error {
	directoryPath := filepath.Dir(doc.path)
	if _, recovered := s.recoveredDirectories[directoryPath]; recovered && !force {
		return nil
	}
	if _, err := os.Stat(directoryPath); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("knowledge store: inspect document directory: %w", err)
	}
	lease, err := advisorylock.AcquireDirectory(ctx, directoryPath)
	if err != nil {
		return fmt.Errorf("knowledge store: acquire document lease: %w", err)
	}
	defer func() { _ = lease.Release() }()
	root, err := os.OpenRoot(doc.root)
	if err != nil {
		return fmt.Errorf("knowledge store: open scope root %q: %w", doc.root, err)
	}
	defer func() { _ = root.Close() }()
	return s.recoverStagedFilesAt(ctx, root, doc, force)
}

func (s *Store) recoverStagedFilesAt(ctx context.Context, root *os.Root, doc document, force bool) error {
	directoryPath := filepath.Dir(doc.path)
	if _, recovered := s.recoveredDirectories[directoryPath]; recovered && !force {
		return nil
	}
	if err := removeStagedFiles(ctx, root, doc.relative); err != nil {
		return fmt.Errorf("knowledge store: recover staged files: %w", err)
	}
	s.recoveredDirectories[directoryPath] = struct{}{}
	return nil
}

func isStagedFileName(name string) bool {
	suffix, ok := strings.CutPrefix(name, stagedFilePrefix)
	if !ok || len(suffix) < 26 {
		return false
	}
	for _, character := range suffix {
		letter := character >= 'A' && character <= 'Z'
		digit := character >= '2' && character <= '7'
		if !letter && !digit {
			return false
		}
	}
	return true
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
	homeDocument, err := s.documentFor(knowledge.ScopeHome, "")
	if err != nil {
		return nil, fmt.Errorf("knowledge store: resolve home path: %w", err)
	}
	cwdDocument, err := s.documentFor(knowledge.ScopeCWD, cwd)
	if err != nil {
		return nil, fmt.Errorf("knowledge store: resolve cwd path: %w", err)
	}
	projectDocument, err := s.documentFor(knowledge.ScopeProjectRoot, projectRoot)
	if err != nil {
		return nil, fmt.Errorf("knowledge store: resolve project-root path: %w", err)
	}
	targets := []document{homeDocument}
	if projectDocument.path != cwdDocument.path {
		targets = append(targets, projectDocument)
	}
	targets = append(targets, cwdDocument)

	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]knowledge.Entry, 0, len(targets))
	for _, target := range targets {
		entry, _, err := s.read(ctx, target)
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, nil
}
