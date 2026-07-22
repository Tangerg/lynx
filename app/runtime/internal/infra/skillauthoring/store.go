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
	"strings"
	"sync"

	skillspec "github.com/Tangerg/lynx/skills"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
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
			return skills.DraftHandle{}, fmt.Errorf("%w: digest collision for revision %q", skills.ErrDraftChanged, handle.Revision)
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
// different active skill is a conflict UNLESS the draft is marked as a revision
// (frontmatter revises: "true"), in which case it replaces the active skill via
// [Store.replaceActive]. An identical active skill is an idempotent replay and
// the redundant draft is removed.
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
		return fmt.Errorf("%w: %q revision %q", skills.ErrDraftChanged, handle.Name, handle.Revision)
	}
	if err := validateSkill(handle.Name, content); err != nil {
		return err
	}
	// A revision replaces the active skill of the same name (archiving the old
	// version) rather than conflicting; it also handles its own archive slot, so
	// it runs before the archived-conflict guard below.
	if revises, err := draftRevises(content); err != nil {
		return err
	} else if revises {
		return s.replaceActive(ctx, root, handle, content, draftDir)
	}
	if _, statErr := root.Lstat(s.archiveDir(handle.Name)); statErr == nil {
		return fmt.Errorf("%w: archived skill %q", skills.ErrConflict, handle.Name)
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return fmt.Errorf("skillauthoring: inspect archived skill %q: %w", handle.Name, statErr)
	}

	activeDir := s.activeDir(handle.Name)
	if active, exists, readErr := readSkill(root, activeDir); readErr != nil {
		return readErr
	} else if exists {
		if !bytes.Equal(active, content) {
			return fmt.Errorf("%w: active skill %q", skills.ErrConflict, handle.Name)
		}
		if err := root.RemoveAll(draftDir); err != nil {
			return fmt.Errorf("skillauthoring: remove replayed draft %q: %w", handle.Name, err)
		}
		return nil
	}
	if _, statErr := root.Lstat(activeDir); statErr == nil {
		return fmt.Errorf("%w: active path %q", skills.ErrConflict, handle.Name)
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
			return fmt.Errorf("%w: active skill %q", skills.ErrConflict, handle.Name)
		}
		return fmt.Errorf("skillauthoring: promote draft %q: %w", handle.Name, err)
	}
	return nil
}

// draftRevises reports whether staged content is marked as a revision of the
// active skill of the same name (frontmatter metadata revises: "true").
func draftRevises(content []byte) (bool, error) {
	front, _, err := skillspec.Parse(content)
	if err != nil {
		return false, fmt.Errorf("skillauthoring: parse draft frontmatter: %w", err)
	}
	return front.Metadata[skills.MetadataRevises] == skills.MetadataTrue, nil
}

// replaceActive installs a revising draft as the active skill, archiving the
// version it supersedes. It OVERWRITES any older archived version of the same
// name — the single-slot history the module keeps by design (no per-version
// archive; that would be the semver theater the skill model rejects). An
// identical active skill is an idempotent no-op; a revision whose target has
// since vanished simply installs as the current version.
func (s *Store) replaceActive(ctx context.Context, root *os.Root, handle skills.DraftHandle, content []byte, draftDir string) error {
	activeDir := s.activeDir(handle.Name)
	active, exists, err := readSkill(root, activeDir)
	if err != nil {
		return err
	}
	if exists && bytes.Equal(active, content) {
		if err := root.RemoveAll(draftDir); err != nil {
			return fmt.Errorf("skillauthoring: remove replayed draft %q: %w", handle.Name, err)
		}
		return nil
	}
	if err := contextError(ctx, "replace skill"); err != nil {
		return err
	}
	if exists {
		if err := s.archiveActive(root, handle.Name); err != nil {
			return err
		}
	}
	if err := root.Rename(draftDir, activeDir); err != nil {
		return fmt.Errorf("skillauthoring: install revised skill %q: %w", handle.Name, err)
	}
	return nil
}

// archiveActive moves the active skill <name> into _archive/<name>, OVERWRITING
// any older archived version — the single history slot the module keeps. The
// caller holds s.mu and owns root. Shared by the revision-replace path and the
// idle-lifecycle sweep.
func (s *Store) archiveActive(root *os.Root, name string) error {
	archiveDir := s.archiveDir(name)
	if err := root.RemoveAll(archiveDir); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("skillauthoring: clear archive slot for %q: %w", name, err)
	}
	if err := root.MkdirAll(skills.ArchivedSubdir, 0o755); err != nil {
		return fmt.Errorf("skillauthoring: create archive area: %w", err)
	}
	if err := root.Rename(s.activeDir(name), archiveDir); err != nil {
		return fmt.Errorf("skillauthoring: archive skill %q: %w", name, err)
	}
	return nil
}

// Archive moves an active skill out of discovery without deleting it, and drops
// its usage record. Dropping the record — the same thing the idle sweep does on
// auto-archive — makes "a restored skill starts with a fresh grace floor" hold
// no matter which path archived it: without it, a manually archived-then-restored
// agent-authored skill would carry a stale last-used time and be re-archived on
// the next sweep.
func (s *Store) Archive(ctx context.Context, name string) error {
	if err := s.move(ctx, name, s.activeDir(name), s.archiveDir(name), "archive"); err != nil {
		return err
	}
	return s.dropUsage(ctx, name)
}

// Restore moves an archived skill back into the active set.
func (s *Store) Restore(ctx context.Context, name string) error {
	// Drop any leftover usage record BEFORE the move so the restored skill always
	// starts with a fresh grace floor — even if an earlier Archive crashed between
	// its rename and its own dropUsage, leaving a stale record. move + dropUsage
	// are two filesystem operations and cannot be atomic; dropping first makes a
	// crash here either a no-op re-restore (still archived, usage already gone) or
	// a clean fresh floor (moved, usage already gone), never active-with-stale-usage.
	if err := s.dropUsage(ctx, name); err != nil {
		return err
	}
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
				return fmt.Errorf("%w: cannot replay %s %q: %w", skills.ErrConflict, operation, name, err)
			}
			return nil
		}
		if _, destinationErr := root.Lstat(destination); destinationErr == nil {
			return fmt.Errorf("%w: cannot replay %s %q: destination is not a valid skill", skills.ErrConflict, operation, name)
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
		return fmt.Errorf("%w: cannot %s %q", skills.ErrConflict, operation, name)
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
			return fmt.Errorf("%w: cannot %s %q", skills.ErrConflict, operation, name)
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

// ListDrafts enumerates the staged proposals under _drafts/, each identified by
// its content-addressed handle and described by its rendered frontmatter
// (including provenance). A directory whose contents no longer hash to its name
// is skipped as corrupt/tampered; unparseable staged content is skipped rather
// than failing the whole listing. Ordering follows the sorted revision dirs.
// Returns empty when authoring is disabled or nothing is staged.
func (s *Store) ListDrafts(ctx context.Context) ([]skills.DraftInfo, error) {
	if !s.Enabled() {
		return nil, nil
	}
	if err := contextError(ctx, "list drafts"); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	root, err := s.openRoot()
	if err != nil {
		return nil, err
	}
	defer root.Close()

	dirEntries, err := fs.ReadDir(root.FS(), skills.DraftsSubdir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("skillauthoring: list drafts: %w", err)
	}
	var out []skills.DraftInfo
	for _, entry := range dirEntries {
		// Skip the transient .stage-* staging dirs and any non-directory entry.
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		revision := entry.Name()
		content, found, err := readSkill(root, filepath.Join(skills.DraftsSubdir, revision))
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		front, _, err := skillspec.Parse(content)
		if err != nil {
			continue
		}
		handle := skills.NewDraftHandle(front.Name, content)
		if handle.Revision != revision {
			continue
		}
		out = append(out, skills.DraftInfo{
			Handle:        handle,
			Description:   front.Description,
			CreatedBy:     front.Metadata[skills.MetadataCreatedBy],
			SourceSession: front.Metadata[skills.MetadataSourceSession],
			Revises:       front.Metadata[skills.MetadataRevises] == skills.MetadataTrue,
		})
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
		return fmt.Errorf("%w: %q revision %q", skills.ErrDraftChanged, handle.Name, handle.Revision)
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

// writeFile creates path (which must not exist) and writes+fsyncs content. It
// backs both draft staging and the usage sidecar, so its messages name the
// operation neutrally; callers add the "draft"/"usage" context.
func writeFile(root *os.Root, path string, content []byte) (err error) {
	file, err := root.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("skillauthoring: create %q: %w", path, err)
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	if _, err := file.Write(content); err != nil {
		return fmt.Errorf("skillauthoring: write %q: %w", path, err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("skillauthoring: sync %q: %w", path, err)
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
