package sessions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// ErrCheckpointUnavailable reports that a file rollback can't restore the working
// tree — the checkpoint store is disabled or the target
// run has no snapshot.
var (
	ErrCheckpointUnavailable = errors.New("sessions: checkpoint unavailable")
	// ErrCheckpointRestoreIncomplete marks a restore that may already have
	// changed part of the working tree. The durable mutation intent must remain
	// pending so boot recovery can re-drive the operation.
	ErrCheckpointRestoreIncomplete = errors.New("sessions: checkpoint restore may be incomplete")
)

const mutationCleanupTimeout = 5 * time.Second

// RollbackSpec is the rollback intent: which Run to keep to and
// what the rollback rewinds. RestoreFiles restores the working tree to the run
// snapshot; RestoreHistory truncates the chat log to the run boundary. Every
// file restore is recoverable; setting both coordinates the two resources
// through the durable operation log described in §8.5.
type RollbackSpec struct {
	SessionID      string
	ToRunID        string
	RestoreFiles   bool
	RestoreHistory bool
}

type DroppedRun struct {
	Run       run.Run
	UserInput []transcript.ContentBlock
}

type RollbackResult struct {
	Session View
	Dropped []DroppedRun
}

// Rollback executes a session rollback as one guarded operation: it claims
// the single-writer mutation slot (rejecting a rollback under an in-flight run
// as [ErrSessionBusy]) and, for a file restore, the working-tree mutation slot
// too, then resolves the boundary under those guards, restores the working tree
// to the run snapshot, restoring files before durable session state, and applies
// the durable history truncation. It returns the resolved session view with the
// mutation result so callers do not re-read a newer revision.
//
// The guards live with the use case: a file restore's `git reset --hard`
// writes a working tree a sibling session sharing the cwd would race, and that
// sibling's tool writes never take the checkpoint lock, so the mutation must see
// any in-flight run on the tree, not just this session's.
func (c *Coordinator) Rollback(ctx context.Context, spec RollbackSpec) (RollbackResult, error) {
	currentSession, err := c.sessions.Get(ctx, spec.SessionID)
	if err != nil {
		return RollbackResult{}, err
	}
	result := RollbackResult{}

	sessionMutation, err := c.ClaimSessionMutation(spec.SessionID)
	if err != nil {
		return result, err
	}
	defer sessionMutation.Release()

	var cwd string
	if spec.RestoreFiles {
		cwd = currentSession.Workspace().Path()
		workingTreeMutation, claimed := c.ClaimWorkingTreeMutation(cwd)
		if !claimed {
			return result, fmt.Errorf("%w: working tree %q has a run admission in flight", ErrSessionBusy, cwd)
		}
		defer workingTreeMutation.Release()
	}

	resolvedBoundary, err := c.resolveRollbackBoundary(ctx, spec.SessionID, spec.ToRunID)
	if err != nil {
		return result, err
	}
	if spec.RestoreHistory {
		result.Dropped = resolvedBoundary.droppedRuns
	}
	// Every file restore is logged before Git touches the working tree. A reset
	// updates multiple paths and can fail after changing only some of them, so
	// even files-only rollback needs boot recovery. RestoreHistory distinguishes
	// that operation from the cross-resource files+history variant.
	mutationRecorded := spec.RestoreFiles && c.mutations != nil
	if mutationRecorded {
		if err := c.recordMutation(ctx, WorkspaceMutation{
			SessionID: spec.SessionID, CWD: cwd, ToRunID: spec.ToRunID,
			RestoreHistory: spec.RestoreHistory,
		}); err != nil {
			return result, err
		}
	}

	// Errors before reset begins leave the tree unchanged, so their intent can be
	// cleared. ErrCheckpointRestoreIncomplete is different: reset may have
	// changed only part of the tree, and its intent must survive for recovery.
	if err := c.restoreRollbackFiles(ctx, spec, cwd, mutationRecorded); err != nil {
		return result, err
	}

	// The tree is restored now; a durable failure here leaves the intent logged so
	// boot recovery completes the truncation (the tree + history would otherwise
	// disagree).
	if spec.RestoreHistory && len(resolvedBoundary.timeline.Dropped) > 0 {
		if err := c.applyRollback(ctx, spec.SessionID, resolvedBoundary.timeline); err != nil {
			return result, err
		}
	}

	if mutationRecorded {
		if err := c.completeMutationDetached(ctx, spec.SessionID); err != nil {
			return result, err
		}
	}
	result.Session, err = c.view(currentSession, ActivityIdle)
	if err != nil {
		return result, err
	}
	return result, nil
}

type resolvedRollbackBoundary struct {
	timeline    transcript.Boundary
	droppedRuns []DroppedRun
}

func (c *Coordinator) resolveRollbackBoundary(
	ctx context.Context,
	sessionID string,
	toRunID string,
) (resolvedRollbackBoundary, error) {
	items, err := c.transcript.List(ctx, sessionID)
	if err != nil {
		return resolvedRollbackBoundary{}, err
	}
	runs, err := c.runs.ListRuns(ctx, sessionID)
	if err != nil {
		return resolvedRollbackBoundary{}, err
	}
	boundary, err := transcript.TimelineFromRuns(runs).BoundaryAt(toRunID, true)
	if err != nil {
		return resolvedRollbackBoundary{}, err
	}
	return resolvedRollbackBoundary{
		timeline:    boundary,
		droppedRuns: projectDroppedRuns(boundary, runs, transcript.OpeningUserMessagesByRun(items)),
	}, nil
}

func (c *Coordinator) restoreRollbackFiles(
	ctx context.Context,
	spec RollbackSpec,
	cwd string,
	mutationRecorded bool,
) error {
	if !spec.RestoreFiles {
		return nil
	}
	err := c.restore(ctx, spec.SessionID, cwd, spec.ToRunID)
	if err == nil || !mutationRecorded || errors.Is(err, ErrCheckpointRestoreIncomplete) {
		return err
	}
	cleanupErr := c.completeMutationDetached(ctx, spec.SessionID)
	if cleanupErr == nil {
		return err
	}
	return errors.Join(err, fmt.Errorf("sessions: clear failed rollback intent: %w", cleanupErr))
}

func projectDroppedRuns(boundary transcript.Boundary, runs []run.Run, inputs map[string][]transcript.ContentBlock) []DroppedRun {
	byID := make(map[string]run.Run, len(runs))
	for _, run := range runs {
		byID[run.ID()] = run
	}
	out := make([]DroppedRun, 0, len(boundary.Dropped))
	for _, node := range boundary.Dropped {
		out = append(out, DroppedRun{Run: byID[node.ID], UserInput: inputs[node.ID]})
	}
	return out
}

// RecoverWorkspaceMutations re-drives every file rollback a crash left
// unfinished (§8.5): for each logged intent it re-restores the working tree
// (reentrant), conditionally re-applies the durable truncation (idempotent — an
// already-committed cut recomputes an empty boundary), then clears the intent.
// It runs at boot before the server serves, so no run contends for the session
// and the admission guards the live path needs are unnecessary. A failed
// recovery aborts startup (returned loud) rather than serving a session whose
// tree and history disagree.
func (c *Coordinator) RecoverWorkspaceMutations(ctx context.Context) error {
	if c.mutations == nil {
		return nil
	}
	pending, err := c.mutations.ListPending(ctx)
	if err != nil {
		return err
	}
	for _, m := range pending {
		if err := c.recoverRollback(ctx, m); err != nil {
			return fmt.Errorf("recover rollback for session %q: %w", m.SessionID, err)
		}
	}
	return nil
}

func (c *Coordinator) recoverRollback(ctx context.Context, m WorkspaceMutation) error {
	var boundary transcript.Boundary
	if m.RestoreHistory {
		runs, err := c.runs.ListRuns(ctx, m.SessionID)
		if err != nil {
			return err
		}
		boundary, err = transcript.TimelineFromRuns(runs).BoundaryAt(m.ToRunID, true)
		if err != nil {
			return err
		}
	}
	if err := c.restore(ctx, m.SessionID, m.CWD, m.ToRunID); err != nil {
		return err
	}
	if m.RestoreHistory && len(boundary.Dropped) > 0 {
		if err := c.applyRollback(ctx, m.SessionID, boundary); err != nil {
			return err
		}
	}
	return c.completeMutation(ctx, m.SessionID)
}

// recordMutation / completeMutation drive the recoverable operation log,
// no-oping when it is disabled (nil) so a build without the log degrades to a
// best-effort rollback rather than nil-panicking.
func (c *Coordinator) recordMutation(ctx context.Context, m WorkspaceMutation) error {
	if c.mutations == nil {
		return nil
	}
	return c.mutations.Record(ctx, m)
}

func (c *Coordinator) completeMutation(ctx context.Context, sessionID string) error {
	if c.mutations == nil {
		return nil
	}
	return c.mutations.Complete(ctx, sessionID)
}

func (c *Coordinator) completeMutationDetached(ctx context.Context, sessionID string) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mutationCleanupTimeout)
	defer cancel()
	return c.completeMutation(cleanupCtx, sessionID)
}

// restore drives the checkpoint store, mapping a nil store (file checkpoints
// disabled) onto [ErrCheckpointUnavailable] so a build without checkpoints
// rejects file restore rather than nil-panicking.
func (c *Coordinator) restore(ctx context.Context, sessionID, cwd, runID string) error {
	if c.checkpoints == nil {
		return ErrCheckpointUnavailable
	}
	return c.checkpoints.Restore(ctx, sessionID, cwd, runID)
}
