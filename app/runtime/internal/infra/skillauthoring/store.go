// Package skillauthoring owns the governed write side of one Agent Skills
// library. Each proposal name has one atomically replaced current document;
// review references remain immutable content digests. Active lifecycle moves
// never overwrite a destination except the explicit single archive slot.
package skillauthoring

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	skillspec "github.com/Tangerg/lynx/skills"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/advisorylock"
)

// Store serializes writes to one scoped skills root. The same instance must be
// shared by every in-process consumer of that root. A scoped directory lease
// linearizes capacity checks, replacement, lifecycle moves, and review
// decisions across Runtime processes that share the library.
type Store struct {
	root  string
	scope skills.Scope
	mu    sync.RWMutex
}

// NewStore roots the authoring store at one project or user Skill library. An
// empty root or invalid scope disables authoring.
func NewStore(root string, scope skills.Scope) *Store { return &Store{root: root, scope: scope} }

// Enabled reports whether a skills root is configured.
func (s *Store) Enabled() bool {
	return s != nil && s.root != "" && s.scope.Validate() == nil
}

// SubmitProposal validates and stages proposal in its name-owned review slot.
// A new revision supersedes the previous pending revision of that name. It
// returns the exact public file identities changed by the call; replaying the
// same proposal is idempotent and returns no identities.
func (s *Store) SubmitProposal(ctx context.Context, proposal skills.Proposal) (skills.ProposalRef, []string, error) {
	if !s.Enabled() {
		return skills.ProposalRef{}, nil, errors.New("skillauthoring: no scoped skills root configured")
	}
	if err := proposal.Validate(); err != nil {
		return skills.ProposalRef{}, nil, err
	}
	if proposal.Scope != s.scope {
		return skills.ProposalRef{}, nil, fmt.Errorf("skillauthoring: proposal scope %q does not match store scope %q", proposal.Scope, s.scope)
	}
	if issue := proposal.SafetyIssue(); issue != skills.ProposalSafe {
		return skills.ProposalRef{}, nil, proposalSafetyError(proposal.Name, issue)
	}
	content, err := renderProposal(proposal)
	if err != nil {
		return skills.ProposalRef{}, nil, err
	}
	ref := skills.NewProposalRef(s.scope, proposal.Name, content)
	if contextErrorErr := contextError(ctx, "save proposal"); contextErrorErr != nil {
		return skills.ProposalRef{}, nil, contextErrorErr
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	root, cleanup, err := s.openLeasedRoot(ctx, "submit proposal")
	if err != nil {
		return skills.ProposalRef{}, nil, err
	}
	defer cleanup()

	if err := root.MkdirAll(proposalsSubdir, 0o755); err != nil {
		return skills.ProposalRef{}, nil, fmt.Errorf("skillauthoring: create proposal area: %w", err)
	}
	proposalDir := s.proposalDir(ref)
	info, statErr := root.Lstat(proposalDir)
	switch {
	case statErr == nil && !info.IsDir():
		return skills.ProposalRef{}, nil, fmt.Errorf("%w: proposal slot %q is not a directory", skills.ErrConflict, proposal.Name)
	case errors.Is(statErr, fs.ErrNotExist):
		pending, listErr := proposalSlotNames(root)
		if listErr != nil {
			return skills.ProposalRef{}, nil, listErr
		}
		if len(pending) >= skills.MaxPendingProposalsPerScope {
			return skills.ProposalRef{}, nil, fmt.Errorf(
				"%w: scope %q already has %d pending names",
				skills.ErrProposalQueueFull,
				s.scope,
				len(pending),
			)
		}
	case statErr != nil:
		return skills.ProposalRef{}, nil, fmt.Errorf("skillauthoring: inspect proposal slot %q: %w", proposal.Name, statErr)
	}
	if existing, found, readErr := readSkill(root, proposalDir); readErr != nil {
		return skills.ProposalRef{}, nil, readErr
	} else if found {
		if bytes.Equal(existing, content) {
			return ref, nil, nil
		}
	}
	if err := stageProposal(ctx, root, proposalDir, content); err != nil {
		return skills.ProposalRef{}, nil, err
	}
	return ref, s.skillIdentities(proposalDir), nil
}

// ApproveProposal publishes exactly the immutable proposal represented by
// handle. A newer pending revision of the same name makes an older handle
// stale, while the exact revision remains content-bound throughout review.
// A different active skill is a conflict unless the proposal explicitly
// revises it. Returned identities report every public file change committed
// before return, including partial changes on error.
func (s *Store) ApproveProposal(ctx context.Context, ref skills.ProposalRef) ([]string, error) {
	if err := s.validateRef(ref); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, cleanup, err := s.openLeasedRoot(ctx, "approve proposal")
	if err != nil {
		return nil, err
	}
	defer cleanup()

	content, found, err := s.readProposal(root, ref)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("skillauthoring: no proposal %q at revision %q: %w", ref.Name, ref.Revision, skills.ErrNotFound)
	}
	if validateSkillErr := validateSkill(ref.Name, content); validateSkillErr != nil {
		return nil, validateSkillErr
	}
	// A revision replaces the active skill of the same name (archiving the old
	// version) rather than conflicting; it also handles its own archive slot, so
	// it runs before the archived-conflict guard below.
	if revises, proposalRevisesErr := proposalRevises(content); proposalRevisesErr != nil {
		return nil, proposalRevisesErr
	} else if revises {
		return s.replaceActive(ctx, root, ref, content)
	}
	if _, statErr := root.Lstat(s.archiveDir(ref.Name)); statErr == nil {
		return nil, fmt.Errorf("%w: archived skill %q", skills.ErrConflict, ref.Name)
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return nil, fmt.Errorf("skillauthoring: inspect archived skill %q: %w", ref.Name, statErr)
	}

	activeDir := s.activeDir(ref.Name)
	if active, exists, readErr := readSkill(root, activeDir); readErr != nil {
		return nil, readErr
	} else if exists {
		if !bytes.Equal(active, content) {
			return nil, fmt.Errorf("%w: active skill %q", skills.ErrConflict, ref.Name)
		}
		removed, removeProposalErr := removeProposal(root, ref)
		identities := identitiesIf(removed, s.skillIdentities(s.proposalDir(ref)))
		if errors.Is(removeProposalErr, skills.ErrProposalChanged) {
			return nil, nil
		}
		if removeProposalErr != nil {
			return identities, fmt.Errorf("skillauthoring: remove replayed proposal %q: %w", ref.Name, removeProposalErr)
		}
		return identities, nil
	}
	if _, statErr := root.Lstat(activeDir); statErr == nil {
		return nil, fmt.Errorf("%w: active path %q", skills.ErrConflict, ref.Name)
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return nil, fmt.Errorf("skillauthoring: inspect active skill %q: %w", ref.Name, statErr)
	}
	if ensureManagedSkillCapacityErr := ensureManagedSkillCapacity(root); ensureManagedSkillCapacityErr != nil {
		return nil, ensureManagedSkillCapacityErr
	}
	if contextErrorErr := contextError(ctx, "approve proposal"); contextErrorErr != nil {
		return nil, contextErrorErr
	}
	if stageSkillErr := stageSkill(ctx, root, activeDir, content); stageSkillErr != nil {
		return nil, stageSkillErr
	}
	identities := s.skillIdentities(activeDir)
	removed, err := removeProposal(root, ref)
	if errors.Is(err, skills.ErrProposalChanged) {
		// A writer outside the governed Store lease replaced the current
		// proposal. The approved bytes are durable and those newer review bytes
		// must remain pending.
		return identities, nil
	}
	if removed {
		identities = append(identities, s.skillIdentities(s.proposalDir(ref))...)
	}
	if err != nil {
		return distinctPaths(identities), fmt.Errorf("skillauthoring: remove approved proposal %q: %w", ref.Name, err)
	}
	return distinctPaths(identities), nil
}

// proposalRevises reports whether staged content is marked as a revision of the
// active skill of the same name (frontmatter metadata revises: "true").
func proposalRevises(content []byte) (bool, error) {
	skill, err := skillspec.Parse(content)
	if err != nil {
		return false, fmt.Errorf("skillauthoring: parse proposal frontmatter: %w", err)
	}
	return skill.Metadata[metadataRevises] == metadataTrue, nil
}

// replaceActive installs a revising proposal as the active skill, archiving the
// version it supersedes. It OVERWRITES any older archived version of the same
// name — the single-slot history the module keeps by design (no per-version
// archive; that would be the semver theater the skill model rejects). An
// identical active skill is an idempotent no-op; a revision whose target has
// since vanished simply installs as the current version.
func (s *Store) replaceActive(ctx context.Context, root *os.Root, ref skills.ProposalRef, content []byte) ([]string, error) {
	activeDir := s.activeDir(ref.Name)
	active, exists, err := readSkill(root, activeDir)
	if err != nil {
		return nil, err
	}
	if exists && bytes.Equal(active, content) {
		removed, removeProposalErr := removeProposal(root, ref)
		identities := identitiesIf(removed, s.skillIdentities(s.proposalDir(ref)))
		if errors.Is(removeProposalErr, skills.ErrProposalChanged) {
			return nil, nil
		}
		if removeProposalErr != nil {
			return identities, fmt.Errorf("skillauthoring: remove replayed proposal %q: %w", ref.Name, removeProposalErr)
		}
		return identities, nil
	}
	if contextErrorErr := contextError(ctx, "replace skill"); contextErrorErr != nil {
		return nil, contextErrorErr
	}
	needsSlot := !exists
	if exists {
		if _, statErr := root.Lstat(s.archiveDir(ref.Name)); errors.Is(statErr, fs.ErrNotExist) {
			needsSlot = true
		} else if statErr != nil {
			return nil, fmt.Errorf("skillauthoring: inspect archived revision slot for %q: %w", ref.Name, statErr)
		}
	}
	if needsSlot {
		if ensureManagedSkillCapacityErr := ensureManagedSkillCapacity(root); ensureManagedSkillCapacityErr != nil {
			return nil, ensureManagedSkillCapacityErr
		}
	}
	var identities []string
	if exists {
		archived, archiveActiveErr := s.archiveActive(root, ref.Name)
		identities = append(identities, archived...)
		if archiveActiveErr != nil {
			return identities, archiveActiveErr
		}
	}
	if stageSkillErr := stageSkill(ctx, root, activeDir, content); stageSkillErr != nil {
		return identities, fmt.Errorf("skillauthoring: install revised skill %q: %w", ref.Name, stageSkillErr)
	}
	identities = append(identities, s.skillIdentities(activeDir)...)
	removed, err := removeProposal(root, ref)
	if errors.Is(err, skills.ErrProposalChanged) {
		return distinctPaths(identities), nil
	}
	if removed {
		identities = append(identities, s.skillIdentities(s.proposalDir(ref))...)
	}
	if err != nil {
		return distinctPaths(identities), fmt.Errorf("skillauthoring: remove revised proposal %q: %w", ref.Name, err)
	}
	return distinctPaths(identities), nil
}

// archiveActive moves the active skill <name> into _archive/<name>, OVERWRITING
// any older archived version — the single history slot the module keeps. The
// caller holds s.mu and owns root. Shared by the revision-replace path and the
// idle-lifecycle sweep.
func (s *Store) archiveActive(root *os.Root, name string) ([]string, error) {
	activeDir := s.activeDir(name)
	content, found, err := readSkill(root, activeDir)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("%w: cannot archive %q", skills.ErrNotFound, name)
	}
	archiveDir := s.archiveDir(name)
	removed, err := removeSkillTree(root, archiveDir)
	identities := identitiesIf(removed, s.skillIdentities(archiveDir))
	if err != nil {
		return identities, fmt.Errorf("skillauthoring: clear archive slot for %q: %w", name, err)
	}
	if err := root.MkdirAll(archivedSubdir, 0o755); err != nil {
		return identities, fmt.Errorf("skillauthoring: create archive area: %w", err)
	}
	if err := root.Rename(activeDir, archiveDir); err != nil {
		moved, reconcileErr := reconcileLifecycleRename(root, name, activeDir, archiveDir, "archive", content, err)
		if moved {
			return distinctPaths(append(identities, s.skillIdentities(activeDir, archiveDir)...)), reconcileErr
		}
		return identities, reconcileErr
	}
	return distinctPaths(append(identities, s.skillIdentities(activeDir, archiveDir)...)), nil
}

// Archive moves an active skill out of discovery without deleting it, returns
// the exact public file identities changed by the move, and drops
// its usage record. Dropping the record — the same thing the idle sweep does on
// auto-archive — makes "a restored skill starts with a fresh grace floor" hold
// no matter which path archived it: without it, a manually archived-then-restored
// agent-authored skill would carry a stale last-used time and be re-archived on
// the next sweep.
func (s *Store) Archive(ctx context.Context, name string) ([]string, error) {
	identities, err := s.moveLifecycle(ctx, name, skills.Active, skills.Archived)
	if err != nil {
		return identities, err
	}
	return identities, s.dropUsage(ctx, name)
}

// Restore moves an archived skill back into the active set and returns the
// exact public file identities changed by the move.
func (s *Store) Restore(ctx context.Context, name string) ([]string, error) {
	// Drop any leftover usage record BEFORE the move so the restored skill always
	// starts with a fresh grace floor — even if an earlier Archive crashed between
	// its rename and its own dropUsage, leaving a stale record. move + dropUsage
	// are two filesystem operations and cannot be atomic; dropping first makes a
	// crash here either a no-op re-restore (still archived, usage already gone) or
	// a clean fresh floor (moved, usage already gone), never active-with-stale-usage.
	if err := s.dropUsage(ctx, name); err != nil {
		return nil, err
	}
	return s.moveLifecycle(ctx, name, skills.Archived, skills.Active)
}

func (s *Store) moveLifecycle(ctx context.Context, name string, from, to skills.Lifecycle) ([]string, error) {
	if !s.Enabled() {
		return nil, errors.New("skillauthoring: no skills root configured")
	}
	if !validName(name) {
		return nil, fmt.Errorf("skillauthoring: invalid skill name %q", name)
	}
	operation, err := lifecycleOperation(from, to)
	if err != nil {
		return nil, err
	}
	source, err := s.lifecycleDir(from, name)
	if err != nil {
		return nil, err
	}
	destination, err := s.lifecycleDir(to, name)
	if err != nil {
		return nil, err
	}
	if contextErrorErr := contextError(ctx, operation+" skill"); contextErrorErr != nil {
		return nil, contextErrorErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, cleanup, err := s.openLeasedRoot(ctx, operation+" skill")
	if err != nil {
		return nil, err
	}
	defer cleanup()

	info, err := root.Lstat(source)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, inspectCompletedLifecycleMove(root, name, destination, operation)
	}
	if err != nil {
		return nil, fmt.Errorf("skillauthoring: cannot %s %q: %w", operation, name, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("skillauthoring: cannot %s %q: source is not a directory", operation, name)
	}
	content, found, err := readSkill(root, source)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("%w: cannot %s %q", skills.ErrNotFound, operation, name)
	}
	if err := validateSkill(name, content); err != nil {
		return nil, err
	}
	if _, err := root.Lstat(destination); err == nil {
		return nil, fmt.Errorf("%w: cannot %s %q", skills.ErrConflict, operation, name)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("skillauthoring: inspect %s destination for %q: %w", operation, name, err)
	}
	if err := root.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return nil, fmt.Errorf("skillauthoring: prepare %s destination for %q: %w", operation, name, err)
	}
	if err := contextError(ctx, operation+" skill"); err != nil {
		return nil, err
	}
	if err := root.Rename(source, destination); err != nil {
		moved, reconcileErr := reconcileLifecycleRename(root, name, source, destination, operation, content, err)
		if moved {
			return s.skillIdentities(source, destination), reconcileErr
		}
		return nil, reconcileErr
	}
	return s.skillIdentities(source, destination), nil
}

func inspectCompletedLifecycleMove(root *os.Root, name, destination, operation string) error {
	content, found, err := readSkill(root, destination)
	if err != nil {
		return fmt.Errorf("skillauthoring: inspect completed %s for %q: %w", operation, name, err)
	}
	if found {
		if err := validateSkill(name, content); err != nil {
			return fmt.Errorf("%w: cannot replay %s %q: %w", skills.ErrConflict, operation, name, err)
		}
		return nil
	}
	if _, err := root.Lstat(destination); err == nil {
		return fmt.Errorf(
			"%w: cannot replay %s %q: destination is not a valid skill",
			skills.ErrConflict,
			operation,
			name,
		)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("skillauthoring: inspect %s destination for %q: %w", operation, name, err)
	}
	return fmt.Errorf("%w: cannot %s %q", skills.ErrNotFound, operation, name)
}

func reconcileLifecycleRename(
	root *os.Root,
	name string,
	source string,
	destination string,
	operation string,
	content []byte,
	renameErr error,
) (bool, error) {
	moved, found, readErr := readSkill(root, destination)
	if readErr != nil {
		return false, fmt.Errorf(
			"skillauthoring: inspect %s outcome for %q: %w",
			operation,
			name,
			errors.Join(renameErr, readErr),
		)
	}
	if found && bytes.Equal(moved, content) {
		if _, sourceErr := root.Lstat(source); errors.Is(sourceErr, fs.ErrNotExist) {
			return true, nil
		} else if sourceErr != nil {
			return false, fmt.Errorf(
				"skillauthoring: inspect %s source for %q: %w",
				operation,
				name,
				sourceErr,
			)
		}
	}
	if _, err := root.Lstat(destination); err == nil {
		return false, fmt.Errorf("%w: cannot %s %q", skills.ErrConflict, operation, name)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false, fmt.Errorf(
			"skillauthoring: inspect %s destination for %q: %w",
			operation,
			name,
			errors.Join(renameErr, err),
		)
	}
	return false, fmt.Errorf("skillauthoring: %s %q: %w", operation, name, renameErr)
}

func lifecycleOperation(from, to skills.Lifecycle) (string, error) {
	switch {
	case from == skills.Active && to == skills.Archived:
		return "archive", nil
	case from == skills.Archived && to == skills.Active:
		return "restore", nil
	default:
		return "", fmt.Errorf("skillauthoring: unsupported lifecycle transition %q to %q", from, to)
	}
}

// List returns active and archived skills from one ordered library snapshot.
func (s *Store) List(ctx context.Context) ([]skills.Entry, error) {
	if !s.Enabled() {
		return nil, nil
	}
	if err := contextError(ctx, "list managed skills"); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	root, cleanup, err := s.openLeasedRoot(ctx, "list managed skills")
	if err != nil {
		return nil, err
	}
	defer cleanup()
	activeNames, archivedNames, err := managedSkillNames(root)
	if err != nil {
		return nil, err
	}
	active, err := managedEntries(ctx, root, ".", activeNames, skills.Active)
	if err != nil {
		return nil, err
	}
	archived, err := managedEntries(ctx, root, archivedSubdir, archivedNames, skills.Archived)
	if err != nil {
		return nil, err
	}
	return append(active, archived...), nil
}

func managedEntries(ctx context.Context, root *os.Root, directory string, names []string, lifecycle skills.Lifecycle) ([]skills.Entry, error) {
	out := make([]skills.Entry, 0, len(names))
	for _, name := range names {
		if err := contextError(ctx, "list managed skills"); err != nil {
			return nil, err
		}
		content, found, err := readSkill(root, filepath.Join(directory, name))
		if err != nil {
			return nil, fmt.Errorf("skillauthoring: list %s skill %q: %w", lifecycle, name, err)
		}
		if !found {
			continue
		}
		skill, err := skillspec.Parse(content)
		if err != nil || skill.Name != name {
			continue
		}
		out = append(out, skills.Entry{Name: name, Description: skill.Description, Lifecycle: lifecycle})
	}
	return out, nil
}

func ensureManagedSkillCapacity(root *os.Root) error {
	active, archived, err := managedSkillNames(root)
	if err != nil {
		return err
	}
	if len(active)+len(archived) >= skills.MaxSkillsPerSource {
		return fmt.Errorf(
			"%w: scope already contains %d managed Skills",
			skills.ErrLibraryCapacity,
			len(active)+len(archived),
		)
	}
	return nil
}

func managedSkillNames(root *os.Root) ([]string, []string, error) {
	active, err := managedSkillNamesAt(root, ".")
	if err != nil {
		return nil, nil, err
	}
	archived, err := managedSkillNamesAt(root, archivedSubdir)
	if err != nil {
		return nil, nil, err
	}
	if len(active)+len(archived) > skills.MaxSkillsPerSource {
		return nil, nil, fmt.Errorf(
			"%w: scope contains %d active and archived Skills; limit is %d",
			skills.ErrLibraryCapacity,
			len(active)+len(archived),
			skills.MaxSkillsPerSource,
		)
	}
	return active, archived, nil
}

func managedSkillNamesAt(root *os.Root, path string) ([]string, error) {
	directory, err := root.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("skillauthoring: list managed skills at %q: %w", path, err)
	}
	entries, readErr := directory.ReadDir(skills.MaxSkillDirectoryEntries + 1)
	if errors.Is(readErr, io.EOF) {
		readErr = nil
	}
	closeErr := directory.Close()
	if readErr != nil || closeErr != nil {
		return nil, fmt.Errorf("skillauthoring: list managed skills at %q: %w", path, errors.Join(readErr, closeErr))
	}
	if len(entries) > skills.MaxSkillDirectoryEntries {
		return nil, fmt.Errorf(
			"%w: %q contains more than %d directory entries",
			skills.ErrLibraryCapacity,
			path,
			skills.MaxSkillDirectoryEntries,
		)
	}
	names := make([]string, 0, min(len(entries), skills.MaxSkillsPerSource))
	for _, entry := range entries {
		if !entry.IsDir() || !validName(entry.Name()) {
			continue
		}
		if len(names) == skills.MaxSkillsPerSource {
			return nil, fmt.Errorf(
				"%w: %q contains more than %d Skills",
				skills.ErrLibraryCapacity,
				path,
				skills.MaxSkillsPerSource,
			)
		}
		names = append(names, entry.Name())
	}
	slices.Sort(names)
	return names, nil
}

// ListProposals enumerates the one current proposal for each name under
// _proposals/. The queue is bounded, so this complete, non-paginated read has a
// finite document and collection cost. Unparseable or path/name-mismatched
// content is skipped; an over-capacity queue fails closed instead of silently
// truncating the review surface. Ordering follows skill name.
func (s *Store) ListProposals(ctx context.Context) ([]skills.ProposalReview, error) {
	if !s.Enabled() {
		return nil, nil
	}
	if err := contextError(ctx, "list proposals"); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	root, cleanup, err := s.openLeasedRoot(ctx, "list proposals")
	if err != nil {
		return nil, err
	}
	defer cleanup()

	names, err := proposalSlotNames(root)
	if err != nil {
		return nil, err
	}
	var out []skills.ProposalReview
	for _, name := range names {
		content, found, err := readSkill(root, filepath.Join(proposalsSubdir, name))
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		skill, err := skillspec.Parse(content)
		if err != nil {
			continue
		}
		origin := skills.ProposalOrigin(skill.Metadata[metadataOrigin])
		if origin != "" && origin.Validate() != nil {
			continue
		}
		ref := skills.NewProposalRef(s.scope, skill.Name, content)
		if ref.Name != name {
			continue
		}
		out = append(out, skills.ProposalReview{
			Ref:           ref,
			Description:   skill.Description,
			Instructions:  skill.Instructions,
			Origin:        origin,
			SourceSession: skill.Metadata[metadataSourceSession],
			Revises:       skill.Metadata[metadataRevises] == metadataTrue,
		})
	}
	return out, nil
}

// proposalSlotNames reads at most the queue capacity plus one directory entry.
// Counting every entry (including malformed external additions) prevents junk
// from bypassing the finite scan; only valid named directories become reviews.
func proposalSlotNames(root *os.Root) ([]string, error) {
	directory, err := root.Open(proposalsSubdir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("skillauthoring: list proposals: %w", err)
	}
	entries, readErr := directory.ReadDir(skills.MaxPendingProposalsPerScope + 1)
	if errors.Is(readErr, io.EOF) {
		readErr = nil
	}
	closeErr := directory.Close()
	if readErr != nil || closeErr != nil {
		return nil, fmt.Errorf("skillauthoring: list proposals: %w", errors.Join(readErr, closeErr))
	}
	if len(entries) > skills.MaxPendingProposalsPerScope {
		return nil, fmt.Errorf(
			"%w: scope contains more than %d entries",
			skills.ErrProposalQueueFull,
			skills.MaxPendingProposalsPerScope,
		)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && validName(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	slices.Sort(names)
	return names, nil
}

// RejectProposal removes only the immutable proposal represented by handle and
// returns its changed public file identity. A missing proposal is already
// discarded; changed bytes are never deleted.
func (s *Store) RejectProposal(ctx context.Context, ref skills.ProposalRef) ([]string, error) {
	if err := s.validateRef(ref); err != nil {
		return nil, err
	}
	if err := contextError(ctx, "reject proposal"); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, cleanup, err := s.openLeasedRoot(ctx, "reject proposal")
	if err != nil {
		return nil, err
	}
	defer cleanup()

	_, found, err := s.readProposal(root, ref)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	removed, err := removeProposal(root, ref)
	identities := identitiesIf(removed, s.skillIdentities(s.proposalDir(ref)))
	if err != nil {
		return identities, fmt.Errorf("skillauthoring: reject proposal %q: %w", ref.Name, err)
	}
	return identities, nil
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

func (s *Store) openLeasedRoot(ctx context.Context, operation string) (*os.Root, func(), error) {
	root, err := s.openRoot()
	if err != nil {
		return nil, nil, err
	}
	lease, err := advisorylock.AcquireDirectory(ctx, s.root)
	if err != nil {
		_ = root.Close()
		return nil, nil, fmt.Errorf("skillauthoring: %s: acquire library lease: %w", operation, err)
	}
	cleanup := func() {
		_ = lease.Release()
		_ = root.Close()
	}
	return root, cleanup, nil
}

func (s *Store) validateRef(ref skills.ProposalRef) error {
	if !s.Enabled() {
		return errors.New("skillauthoring: no scoped skills root configured")
	}
	if err := ref.Validate(); err != nil {
		return fmt.Errorf("skillauthoring: invalid proposal reference: %w", err)
	}
	if ref.Scope != s.scope {
		return fmt.Errorf("skillauthoring: proposal scope %q does not match store scope %q", ref.Scope, s.scope)
	}
	if !validName(ref.Name) {
		return fmt.Errorf("skillauthoring: invalid skill name %q", ref.Name)
	}
	return nil
}

func (s *Store) activeDir(name string) string { return name }

func (s *Store) archiveDir(name string) string {
	return filepath.Join(archivedSubdir, name)
}

func (s *Store) proposalDir(ref skills.ProposalRef) string {
	return filepath.Join(proposalsSubdir, ref.Name)
}

func (s *Store) skillIdentities(directories ...string) []string {
	identities := make([]string, 0, len(directories))
	for _, directory := range directories {
		identities = append(identities, filepath.Join(s.root, directory, skillspec.SkillFile))
	}
	return distinctPaths(identities)
}

func distinctPaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if path != "" && !slices.Contains(out, path) {
			out = append(out, path)
		}
	}
	return out
}

func identitiesIf(changed bool, identities []string) []string {
	if !changed {
		return nil
	}
	return identities
}

func removeSkillTree(root *os.Root, directory string) (bool, error) {
	_, existed, err := readSkill(root, directory)
	if err != nil {
		return false, err
	}
	removeErr := root.RemoveAll(directory)
	if removeErr == nil || errors.Is(removeErr, fs.ErrNotExist) {
		return existed, nil
	}
	_, remains, inspectErr := readSkill(root, directory)
	return existed && !remains, errors.Join(removeErr, inspectErr)
}

func (s *Store) lifecycleDir(lifecycle skills.Lifecycle, name string) (string, error) {
	switch lifecycle {
	case skills.Active:
		return s.activeDir(name), nil
	case skills.Archived:
		return s.archiveDir(name), nil
	default:
		return "", fmt.Errorf("skillauthoring: unknown lifecycle %q", lifecycle)
	}
}

func (s *Store) readProposal(root *os.Root, ref skills.ProposalRef) ([]byte, bool, error) {
	content, found, err := readSkill(root, s.proposalDir(ref))
	if err != nil || !found {
		return content, found, err
	}
	if !ref.Matches(content) {
		return nil, false, fmt.Errorf("%w: %q revision %q", skills.ErrProposalChanged, ref.Name, ref.Revision)
	}
	return content, true, nil
}

func validateSkill(name string, content []byte) error {
	if len(content) > skills.MaxAuthoredSkillDocumentBytes {
		return fmt.Errorf(
			"skillauthoring: validate skill %q: %w: %d bytes exceeds %d",
			name,
			skills.ErrDocumentTooLarge,
			len(content),
			skills.MaxAuthoredSkillDocumentBytes,
		)
	}
	skill, err := skillspec.Parse(content)
	if err != nil {
		return fmt.Errorf("skillauthoring: parse skill %q: %w", name, err)
	}
	if strings.TrimSpace(skill.Instructions) == "" {
		return fmt.Errorf("skillauthoring: validate skill %q: skill instructions are required", name)
	}
	if skill.Name != name {
		return fmt.Errorf("skillauthoring: skill name mismatch: frontmatter %q, path %q", skill.Name, name)
	}
	proposal := skills.Proposal{Name: skill.Name, Description: skill.Description, Instructions: skill.Instructions}
	if issue := proposal.SafetyIssue(); issue != skills.ProposalSafe {
		return proposalSafetyError(name, issue)
	}
	return nil
}

func proposalSafetyError(name string, issue skills.ProposalSafetyIssue) error {
	switch issue {
	case skills.ProposalDangerousInstruction:
		return fmt.Errorf("skillauthoring: reject skill %q: dangerous instruction", name)
	default:
		return fmt.Errorf("skillauthoring: reject skill %q: unknown safety issue", name)
	}
}

func contextError(ctx context.Context, operation string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("skillauthoring: %s: %w", operation, err)
	}
	return nil
}

func validName(name string) bool {
	return skillspec.ValidateName(name) == nil
}
