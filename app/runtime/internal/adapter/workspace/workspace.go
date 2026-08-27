// Package workspace adapts git-backed workspace operations: the VCS view of a
// project's working tree (git-backed diff / status) and per-session file
// checkpoints (shadow-git snapshot / restore). It owns the Git and checkpoint
// implementations behind those semantic operations.
//
// VCS reads are stateless package functions (a git working tree is addressed
// purely by its root path); checkpoint lifecycle is stateful and lives on
// [Checkpoints], which holds the shadow-repo store.
package workspace

import (
	"context"

	"github.com/Tangerg/scope/app/runtime/internal/infra/checkpoint"
	"github.com/Tangerg/scope/app/runtime/internal/infra/git"
)

var (
	// ErrCheckpointUnavailable means the file-checkpoint store is disabled (git
	// absent) or holds no snapshot for the target run. Callers translate this
	// sentinel at their own boundary.
	ErrCheckpointUnavailable = checkpoint.ErrUnavailable
	// ErrCheckpointRestoreIncomplete means Git started a work-tree reset but did
	// not finish; orchestration must keep its durable recovery intent.
	ErrCheckpointRestoreIncomplete = checkpoint.ErrRestoreIncomplete
)

// GitAvailable reports whether the git binary is on PATH — gates the
// workspace VCS + checkpoint features.
func GitAvailable() bool { return git.Available() }

// ListChanges scans root's working tree for changed files.
func ListChanges(ctx context.Context, root string, maxChanges int) ([]git.FileChange, error) {
	return git.ListChanges(ctx, root, maxChanges)
}

// Diff returns the structured diff for root (optionally scoped to relPath).
// base selects the diff target: false = working tree, true = merge-base with
// the default branch.
func Diff(ctx context.Context, root, relPath string, base bool, maxFiles, maxRows, maxBytes int) ([]git.DiffFile, bool, error) {
	return git.Diff(ctx, root, relPath, diffMode(base), maxFiles, maxRows, maxBytes)
}

// RawDiff returns the unified patch text for root (optionally scoped to
// relPath). base selects the diff target as in [Diff].
func RawDiff(ctx context.Context, root, relPath string, base bool, maxBytes int) (string, error) {
	return git.RawDiff(ctx, root, relPath, diffMode(base), maxBytes)
}

func diffMode(base bool) git.Mode {
	if base {
		return git.Base
	}
	return git.Worktree
}

// Checkpoints owns the per-session file-checkpoint lifecycle over a shadow-git
// store. A nil Checkpoints (or one built with checkpoints disabled) reports
// CheckpointsEnabled false and treats every operation as the unavailable path.
type Checkpoints struct {
	store *checkpoint.Store // nil when git is unavailable / no dir
}

// NewCheckpoints builds the checkpoint adapter. File checkpoints are enabled
// only when the git binary is present and checkpointDir is non-empty; otherwise
// the store is nil and snapshot/restore degrade to the unavailable path.
func NewCheckpoints(checkpointDir string) *Checkpoints {
	var store *checkpoint.Store
	if checkpointDir != "" && git.Available() {
		store = checkpoint.NewStore(checkpointDir)
	}
	return &Checkpoints{store: store}
}

// CheckpointsEnabled reports whether file checkpoints are available — backs
// the features.checkpoints flag. Nil-safe.
func (c *Checkpoints) CheckpointsEnabled() bool {
	return c != nil && c.store != nil
}

// Snapshot anchors sessionID's working tree (at cwd) under runID so a later
// Restore can revert to it. Best-effort: a disabled store is a silent no-op,
// so the caller never fails a run on snapshot trouble. Nil-safe.
func (c *Checkpoints) Snapshot(ctx context.Context, sessionID, cwd, runID string) error {
	if !c.CheckpointsEnabled() {
		return nil
	}
	// Only checkpoint a real git repo — state tracking requires git.
	// A repo's own .gitignore bounds what the whole-tree `git add` stages; a
	// non-repo dir (e.g. a session opened on the home directory) has no such
	// bound, so snapshotting it would try to stage the entire tree — minutes of
	// `git add` on millions of files. Non-git projects get no file checkpoint,
	// by design: file rollback is a git-shaped feature, only safe where a
	// .gitignore scopes the work tree.
	repository, err := git.IsRepo(ctx, cwd)
	if err != nil {
		// Snapshot failures are best-effort, but cancellation is the caller's
		// control signal rather than a checkpoint diagnostic.
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		return nil
	}
	if !repository {
		return nil
	}
	return c.store.Snapshot(ctx, sessionID, cwd, runID)
}

// Restore resets sessionID's working tree (at cwd) to the runID snapshot. A
// disabled store or missing snapshot surfaces as [ErrCheckpointUnavailable]; a
// failed reset that may have changed part of the tree surfaces as
// [ErrCheckpointRestoreIncomplete]. Nil-safe.
func (c *Checkpoints) Restore(ctx context.Context, sessionID, cwd, runID string) error {
	if !c.CheckpointsEnabled() {
		return ErrCheckpointUnavailable
	}
	return c.store.Restore(ctx, sessionID, cwd, runID)
}

// DropSession removes a session's shadow repo (on session delete).
// Best-effort, nil-safe no-op when checkpoints are disabled.
func (c *Checkpoints) DropSession(sessionID string) error {
	if !c.CheckpointsEnabled() {
		return nil
	}
	return c.store.DropSession(sessionID)
}
