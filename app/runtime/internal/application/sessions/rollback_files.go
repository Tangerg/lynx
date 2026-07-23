package sessions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// ErrCheckpointUnavailable reports that a file rollback can't restore the working
// tree — the checkpoint store is disabled, the session has no cwd, or the target
// run has no snapshot. The composition root maps the checkpoint adapter's own
// sentinel onto this one so the coordinator stays free of the adapter package.
var (
	ErrCheckpointUnavailable = errors.New("sessions: checkpoint unavailable")
	// ErrCheckpointRestoreIncomplete marks a restore that may already have
	// changed part of the working tree. The durable mutation intent must remain
	// pending so boot recovery can re-drive the operation.
	ErrCheckpointRestoreIncomplete = errors.New("sessions: checkpoint restore may be incomplete")
)

const mutationCleanupTimeout = 5 * time.Second

// RollbackSpec is the wire-decoded rollback intent: which run to keep to and
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
	Run       transcript.Run
	UserInput []transcript.ContentBlock
}

type RollbackResult struct {
	Session session.Session
	Dropped []DroppedRun
}

// RollbackFiles executes a session rollback as one guarded operation: it claims
// the single-writer mutation slot (rejecting a rollback under an in-flight run
// as [ErrSessionBusy]) and, for a file restore, the working-tree mutation slot
// too, then resolves the boundary under those guards, restores the working tree
// to the run snapshot (files first, as required by AUX_API §4.1), and applies
// the durable history truncation. It returns the
// session so the delivery adapter can shape its response without re-reading it.
//
// The guards live here, not at the wire: a file restore's `git reset --hard`
// writes a working tree a sibling session sharing the cwd would race, and that
// sibling's tool writes never take the checkpoint lock, so the mutation must see
// any in-flight run on the tree (ActiveSessionWithCwd), not just this session's.
func (c *Coordinator) RollbackFiles(ctx context.Context, spec RollbackSpec) (RollbackResult, error) {
	ses, err := c.s.Session().Get(ctx, spec.SessionID)
	if err != nil {
		return RollbackResult{}, err
	}
	result := RollbackResult{Session: ses}

	admission, err := c.ClaimMutationSlot(spec.SessionID)
	if err != nil {
		return result, err
	}
	defer admission.Release()

	var cwd string
	if spec.RestoreFiles {
		cwd = ses.Cwd
		if cwd == "" {
			return result, ErrCheckpointUnavailable
		}
		treeAdmission, ok := c.ClaimWorkingTreeMutation(cwd)
		if !ok {
			return result, fmt.Errorf("%w: working tree %q has a run admission in flight", ErrSessionBusy, cwd)
		}
		defer treeAdmission.Release()
		if busy := c.admissions.ActiveSessionWithCwd(cwd); busy != "" {
			return result, fmt.Errorf("%w: session %q shares this working tree and has a run in flight", ErrSessionBusy, busy)
		}
	}

	items, runs, err := c.s.Transcript().List(ctx, spec.SessionID)
	if err != nil {
		return result, err
	}
	boundary, err := transcript.TimelineFromRuns(runs).BoundaryAt(spec.ToRunID, true)
	if err != nil {
		return result, err
	}
	if spec.RestoreHistory {
		result.Dropped = droppedRuns(boundary, runs, transcript.OpeningInputs(items))
	}
	// Resolve and claim the complete internal session subtree before restoring
	// files. Waiting until the SQLite phase would let a child run reject the
	// history delete only after the working tree had already been rewound.
	var dropSessionIDs []string
	var childAdmissions []RunAdmission
	if spec.RestoreHistory {
		dropSessionIDs, childAdmissions, err = c.prepareRollbackSessions(ctx, spec.SessionID, boundary)
		if err != nil {
			return result, err
		}
	}
	defer releaseAdmissions(childAdmissions)

	// Every file restore is logged before Git touches the working tree. A reset
	// updates multiple paths and can fail after changing only some of them, so
	// even files-only rollback needs boot recovery. RestoreHistory distinguishes
	// that operation from the cross-resource files+history variant.
	restoreLogged := spec.RestoreFiles && c.mutations != nil
	if restoreLogged {
		if err := c.recordMutation(ctx, execution.WorkspaceMutation{
			SessionID: spec.SessionID, Cwd: cwd, ToRunID: spec.ToRunID,
			RestoreHistory: spec.RestoreHistory,
		}); err != nil {
			return result, err
		}
	}

	// Errors before reset begins leave the tree unchanged, so their intent can be
	// cleared. ErrCheckpointRestoreIncomplete is different: reset may have
	// changed only part of the tree, and its intent must survive for recovery.
	if spec.RestoreFiles {
		if err := c.restore(ctx, spec.SessionID, cwd, spec.ToRunID); err != nil {
			if restoreLogged && !errors.Is(err, ErrCheckpointRestoreIncomplete) {
				if cleanupErr := c.completeMutationDetached(ctx, spec.SessionID); cleanupErr != nil {
					return result, errors.Join(err, fmt.Errorf("sessions: clear failed rollback intent: %w", cleanupErr))
				}
			}
			return result, err
		}
	}

	// The tree is restored now; a durable failure here leaves the intent logged so
	// boot recovery completes the truncation (the tree + history would otherwise
	// disagree).
	if spec.RestoreHistory && len(boundary.Dropped) > 0 {
		if err := c.applyRollback(ctx, spec.SessionID, boundary, dropSessionIDs); err != nil {
			return result, err
		}
	}

	if restoreLogged {
		if err := c.completeMutationDetached(ctx, spec.SessionID); err != nil {
			return result, err
		}
	}
	return result, nil
}

func droppedRuns(boundary transcript.Boundary, runs []transcript.Run, inputs map[string][]transcript.ContentBlock) []DroppedRun {
	byID := make(map[string]transcript.Run, len(runs))
	for _, run := range runs {
		byID[run.ID] = run
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

func (c *Coordinator) recoverRollback(ctx context.Context, m execution.WorkspaceMutation) error {
	var boundary transcript.Boundary
	var dropSessionIDs []string
	if m.RestoreHistory {
		_, runs, err := c.s.Transcript().List(ctx, m.SessionID)
		if err != nil {
			return err
		}
		boundary, err = transcript.TimelineFromRuns(runs).BoundaryAt(m.ToRunID, true)
		if err != nil {
			return err
		}
		if len(boundary.Dropped) > 0 {
			dropSessionIDs, err = c.subtaskSessionsAfter(ctx, m.SessionID, boundary.BoundaryTime)
			if err != nil {
				return err
			}
		}
	}
	if err := c.restore(ctx, m.SessionID, m.Cwd, m.ToRunID); err != nil {
		return err
	}
	if m.RestoreHistory && len(boundary.Dropped) > 0 {
		if err := c.applyRollback(ctx, m.SessionID, boundary, dropSessionIDs); err != nil {
			return err
		}
	}
	return c.completeMutation(ctx, m.SessionID)
}

// recordMutation / completeMutation drive the recoverable operation log,
// no-oping when it is disabled (nil) so a build without the log degrades to a
// best-effort rollback rather than nil-panicking.
func (c *Coordinator) recordMutation(ctx context.Context, m execution.WorkspaceMutation) error {
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
