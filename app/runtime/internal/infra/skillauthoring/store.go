// Package skillauthoring owns the governed write side of the global Agent
// Skills library. Drafts are immutable and content-addressed; lifecycle moves
// never overwrite an existing directory.
package skillauthoring

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	skillspec "github.com/Tangerg/lynx/skills"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

var (
	// ErrConflict reports that the destination lifecycle state already contains
	// a different skill. Callers must resolve the conflict explicitly; the store
	// never destroys either side to make a move succeed.
	ErrConflict = errors.New("skillauthoring: destination already exists")
	// ErrDraftChanged reports that staged bytes no longer match the handle that
	// was approved. The store leaves both the draft and active library untouched.
	ErrDraftChanged = errors.New("skillauthoring: staged draft content changed")
)

// Store serializes writes to one global skills root. The same instance must be
// shared by the proposal tool and lifecycle curator so in-process operations
// have one order; no-clobber directory renames preserve data across processes.
type Store struct {
	root string
	mu   sync.RWMutex
}

// NewStore roots the authoring store at the global skills directory. An empty
// root disables authoring and causes write operations to fail explicitly.
func NewStore(root string) *Store { return &Store{root: root} }

// Enabled reports whether a skills root is configured.
func (s *Store) Enabled() bool { return s != nil && s.root != "" }

// SaveDraft validates and stages draft under its content-addressed handle. It
// is idempotent: replaying the same proposal returns the same handle and bytes.
func (s *Store) SaveDraft(ctx context.Context, draft skills.Draft) (skills.DraftHandle, error) {
	if !s.Enabled() {
		return skills.DraftHandle{}, errors.New("skillauthoring: no skills root configured")
	}
	if err := draft.Validate(); err != nil {
		return skills.DraftHandle{}, err
	}
	if reason, dangerous := draft.Scan(); dangerous {
		return skills.DraftHandle{}, fmt.Errorf("skillauthoring: reject draft %q: %s", draft.Name, reason)
	}
	content, err := draft.Render()
	if err != nil {
		return skills.DraftHandle{}, err
	}
	handle := skills.NewDraftHandle(draft.Name, []byte(content))
	if err := contextError(ctx, "save draft"); err != nil {
		return skills.DraftHandle{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	root, err := s.openRoot()
	if err != nil {
		return skills.DraftHandle{}, err
	}
	defer root.Close()

	draftDir := s.draftDir(handle)
	if existing, found, readErr := readSkill(root, draftDir); readErr != nil {
		return skills.DraftHandle{}, readErr
	} else if found {
		if !bytes.Equal(existing, []byte(content)) {
			return skills.DraftHandle{}, fmt.Errorf("%w: digest collision for revision %q", ErrDraftChanged, handle.Revision)
		}
		return handle, nil
	}

	if err := root.MkdirAll(skills.DraftsSubdir, 0o755); err != nil {
		return skills.DraftHandle{}, fmt.Errorf("skillauthoring: create draft area: %w", err)
	}
	if err := stageDraft(ctx, root, draftDir, []byte(content)); err != nil {
		return skills.DraftHandle{}, err
	}
	return handle, nil
}

// Promote publishes exactly the immutable draft represented by handle. A
// different active skill is a conflict; an identical active skill is treated
// as an idempotent replay and the redundant draft is removed.
func (s *Store) Promote(ctx context.Context, handle skills.DraftHandle) error {
	if err := s.validateHandle(handle); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, err := s.openRoot()
	if err != nil {
		return err
	}
	defer root.Close()

	draftDir := s.draftDir(handle)
	content, found, err := readSkill(root, draftDir)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("skillauthoring: no draft %q at revision %q: %w", handle.Name, handle.Revision, fs.ErrNotExist)
	}
	if !handle.Matches(content) {
		return fmt.Errorf("%w: %q revision %q", ErrDraftChanged, handle.Name, handle.Revision)
	}
	if err := validateSkill(handle.Name, content); err != nil {
		return err
	}
	if _, statErr := root.Lstat(s.archiveDir(handle.Name)); statErr == nil {
		return fmt.Errorf("%w: archived skill %q", ErrConflict, handle.Name)
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return fmt.Errorf("skillauthoring: inspect archived skill %q: %w", handle.Name, statErr)
	}

	activeDir := s.activeDir(handle.Name)
	if active, exists, readErr := readSkill(root, activeDir); readErr != nil {
		return readErr
	} else if exists {
		if !bytes.Equal(active, content) {
			return fmt.Errorf("%w: active skill %q", ErrConflict, handle.Name)
		}
		if err := root.RemoveAll(draftDir); err != nil {
			return fmt.Errorf("skillauthoring: remove replayed draft %q: %w", handle.Name, err)
		}
		return nil
	}
	if _, statErr := root.Lstat(activeDir); statErr == nil {
		return fmt.Errorf("%w: active path %q", ErrConflict, handle.Name)
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return fmt.Errorf("skillauthoring: inspect active skill %q: %w", handle.Name, statErr)
	}
	if err := contextError(ctx, "promote draft"); err != nil {
		return err
	}
	if err := root.Rename(draftDir, activeDir); err != nil {
		active, exists, readErr := readSkill(root, activeDir)
		if readErr != nil {
			return fmt.Errorf("skillauthoring: inspect promotion outcome for %q: %w", handle.Name, errors.Join(err, readErr))
		}
		if exists && bytes.Equal(active, content) {
			if removeErr := root.RemoveAll(draftDir); removeErr != nil && !errors.Is(removeErr, fs.ErrNotExist) {
				return fmt.Errorf("skillauthoring: remove replayed draft %q: %w", handle.Name, removeErr)
			}
			return nil
		}
		if _, statErr := root.Lstat(activeDir); statErr == nil {
			return fmt.Errorf("%w: active skill %q", ErrConflict, handle.Name)
		}
		return fmt.Errorf("skillauthoring: promote draft %q: %w", handle.Name, err)
	}
	return nil
}

// Archive moves an active skill out of discovery without deleting it.
func (s *Store) Archive(ctx context.Context, name string) error {
	return s.move(ctx, name, s.activeDir(name), s.archiveDir(name), "archive")
}

// Restore moves an archived skill back into the active set.
func (s *Store) Restore(ctx context.Context, name string) error {
	return s.move(ctx, name, s.archiveDir(name), s.activeDir(name), "restore")
}

func (s *Store) move(ctx context.Context, name, source, destination, operation string) error {
	if !s.Enabled() {
		return errors.New("skillauthoring: no skills root configured")
	}
	if !validName(name) {
		return fmt.Errorf("skillauthoring: invalid skill name %q", name)
	}
	if err := contextError(ctx, operation+" skill"); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, err := s.openRoot()
	if err != nil {
		return err
	}
	defer root.Close()

	info, err := root.Lstat(source)
	if errors.Is(err, fs.ErrNotExist) {
		content, found, readErr := readSkill(root, destination)
		if readErr != nil {
			return fmt.Errorf("skillauthoring: inspect completed %s for %q: %w", operation, name, readErr)
		}
		if found {
			if err := validateSkill(name, content); err != nil {
				return fmt.Errorf("%w: cannot replay %s %q: %w", ErrConflict, operation, name, err)
			}
			return nil
		}
		if _, destinationErr := root.Lstat(destination); destinationErr == nil {
			return fmt.Errorf("%w: cannot replay %s %q: destination is not a valid skill", ErrConflict, operation, name)
		} else if !errors.Is(destinationErr, fs.ErrNotExist) {
			return fmt.Errorf("skillauthoring: inspect %s destination for %q: %w", operation, name, destinationErr)
		}
	}
	if err != nil {
		return fmt.Errorf("skillauthoring: cannot %s %q: %w", operation, name, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("skillauthoring: cannot %s %q: source is not a directory", operation, name)
	}
	content, found, err := readSkill(root, source)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("skillauthoring: cannot %s %q: %w", operation, name, fs.ErrNotExist)
	}
	if err := validateSkill(name, content); err != nil {
		return err
	}
	if _, err := root.Lstat(destination); err == nil {
		return fmt.Errorf("%w: cannot %s %q", ErrConflict, operation, name)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("skillauthoring: inspect %s destination for %q: %w", operation, name, err)
	}
	if err := root.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("skillauthoring: prepare %s destination for %q: %w", operation, name, err)
	}
	if err := contextError(ctx, operation+" skill"); err != nil {
		return err
	}
	if err := root.Rename(source, destination); err != nil {
		moved, found, readErr := readSkill(root, destination)
		if readErr != nil {
			return fmt.Errorf("skillauthoring: inspect %s outcome for %q: %w", operation, name, errors.Join(err, readErr))
		}
		if found && bytes.Equal(moved, content) {
			if _, sourceErr := root.Lstat(source); errors.Is(sourceErr, fs.ErrNotExist) {
				return nil
			} else if sourceErr != nil {
				return fmt.Errorf("skillauthoring: inspect %s source for %q: %w", operation, name, sourceErr)
			}
		}
		if _, statErr := root.Lstat(destination); statErr == nil {
			return fmt.Errorf("%w: cannot %s %q", ErrConflict, operation, name)
		} else if !errors.Is(statErr, fs.ErrNotExist) {
			return fmt.Errorf("skillauthoring: inspect %s destination for %q: %w", operation, name, errors.Join(err, statErr))
		}
		return fmt.Errorf("skillauthoring: %s %q: %w", operation, name, err)
	}
	return nil
}

// List returns active and archived skills from one ordered library snapshot.
func (s *Store) List(ctx context.Context) ([]skills.Entry, error) {
	if !s.Enabled() {
		return nil, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	active, err := entries(ctx, s.root, skills.Active)
	if err != nil {
		return nil, err
	}
	archived, err := entries(ctx, filepath.Join(s.root, skills.ArchivedSubdir), skills.Archived)
	if err != nil {
		return nil, err
	}
	return append(active, archived...), nil
}

func entries(ctx context.Context, dir string, lifecycle skills.Lifecycle) ([]skills.Entry, error) {
	summaries, err := skillspec.Dir(dir).List(ctx)
	if err != nil {
		return nil, fmt.Errorf("skillauthoring: list %s skills: %w", lifecycle, err)
	}
	out := make([]skills.Entry, len(summaries))
	for i, summary := range summaries {
		out[i] = skills.Entry{Name: summary.Name, Description: summary.Description, Lifecycle: lifecycle}
	}
	return out, nil
}

// DiscardDraft removes only the immutable draft represented by handle. A
// missing draft is already discarded; changed bytes are never deleted.
func (s *Store) DiscardDraft(ctx context.Context, handle skills.DraftHandle) error {
	if err := s.validateHandle(handle); err != nil {
		return err
	}
	if err := contextError(ctx, "discard draft"); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, err := s.openRoot()
	if err != nil {
		return err
	}
	defer root.Close()

	draftDir := s.draftDir(handle)
	content, found, err := readSkill(root, draftDir)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if !handle.Matches(content) {
		return fmt.Errorf("%w: %q revision %q", ErrDraftChanged, handle.Name, handle.Revision)
	}
	if err := root.RemoveAll(draftDir); err != nil {
		return fmt.Errorf("skillauthoring: discard draft %q: %w", handle.Name, err)
	}
	return nil
}

func (s *Store) openRoot() (*os.Root, error) {
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return nil, fmt.Errorf("skillauthoring: create skills root: %w", err)
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, fmt.Errorf("skillauthoring: open skills root: %w", err)
	}
	return root, nil
}

func (s *Store) validateHandle(handle skills.DraftHandle) error {
	if !s.Enabled() {
		return errors.New("skillauthoring: no skills root configured")
	}
	if err := handle.Validate(); err != nil {
		return fmt.Errorf("skillauthoring: invalid draft handle: %w", err)
	}
	if !validName(handle.Name) {
		return fmt.Errorf("skillauthoring: invalid skill name %q", handle.Name)
	}
	return nil
}

func (s *Store) activeDir(name string) string { return name }

func (s *Store) archiveDir(name string) string {
	return filepath.Join(skills.ArchivedSubdir, name)
}

func (s *Store) draftDir(handle skills.DraftHandle) string {
	return filepath.Join(skills.DraftsSubdir, handle.Revision)
}

func readSkill(root *os.Root, dir string) ([]byte, bool, error) {
	content, err := root.ReadFile(filepath.Join(dir, skillFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("skillauthoring: read %q: %w", dir, err)
	}
	return content, true, nil
}

func writeFile(root *os.Root, path string, content []byte) (err error) {
	file, err := root.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("skillauthoring: create draft file: %w", err)
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	if _, err := file.Write(content); err != nil {
		return fmt.Errorf("skillauthoring: write draft file: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("skillauthoring: sync draft file: %w", err)
	}
	return nil
}

func stageDraft(ctx context.Context, root *os.Root, destination string, content []byte) (err error) {
	temporary := filepath.Join(skills.DraftsSubdir, ".stage-"+rand.Text())
	if err := root.Mkdir(temporary, 0o755); err != nil {
		return fmt.Errorf("skillauthoring: create draft staging directory: %w", err)
	}
	defer func() {
		if cleanupErr := root.RemoveAll(temporary); cleanupErr != nil && !errors.Is(cleanupErr, fs.ErrNotExist) {
			err = errors.Join(err, fmt.Errorf("skillauthoring: clean draft staging directory: %w", cleanupErr))
		}
	}()
	if err := writeFile(root, filepath.Join(temporary, skillFile), content); err != nil {
		return err
	}
	if err := contextError(ctx, "publish draft"); err != nil {
		return err
	}
	if err := root.Rename(temporary, destination); err != nil {
		existing, found, readErr := readSkill(root, destination)
		if readErr == nil && found && bytes.Equal(existing, content) {
			return nil
		}
		return fmt.Errorf("skillauthoring: publish draft %q: %w", filepath.Base(destination), errors.Join(err, readErr))
	}
	return nil
}

func validateSkill(name string, content []byte) error {
	frontmatter, body, err := skillspec.Parse(content)
	if err != nil {
		return fmt.Errorf("skillauthoring: parse skill %q: %w", name, err)
	}
	draft := skills.Draft{Name: frontmatter.Name, Description: frontmatter.Description, Body: body}
	if err := draft.Validate(); err != nil {
		return fmt.Errorf("skillauthoring: validate skill %q: %w", name, err)
	}
	if frontmatter.Name != name {
		return fmt.Errorf("skillauthoring: skill name mismatch: frontmatter %q, path %q", frontmatter.Name, name)
	}
	if reason, dangerous := draft.Scan(); dangerous {
		return fmt.Errorf("skillauthoring: reject skill %q: %s", name, reason)
	}
	return nil
}

func contextError(ctx context.Context, operation string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("skillauthoring: %s: %w", operation, err)
	}
	return nil
}

const skillFile = "SKILL.md"

func validName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	return name == filepath.Base(name) && !filepath.IsAbs(name)
}
