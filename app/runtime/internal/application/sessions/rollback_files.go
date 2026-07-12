package sessions

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/worktree"
)

// ErrCheckpointUnavailable reports that a file rollback can't restore the working
// tree — the checkpoint store is disabled, the session has no cwd, or the target
// run has no snapshot. The composition root maps the checkpoint adapter's own
// sentinel onto this one so the coordinator stays free of the adapter package.
var ErrCheckpointUnavailable = errors.New("sessions: checkpoint unavailable")

// RollbackSpec is the wire-decoded rollback intent: which run to keep to and
// what the rollback rewinds. RestoreFiles restores the working tree to the run
// snapshot; RestoreHistory truncates the chat log to the run boundary. Both set
// is the atomic files+history rollback whose cross-resource recovery §8.5
// covers.
type RollbackSpec struct {
	SessionID      string
	ToRunID        string
	RestoreFiles   bool
	RestoreHistory bool
}

// BoundaryResolver yields the domain rollback cut for the target run, resolved
// UNDER the claimed mutation slot so no concurrent run can append to the
// transcript between the read and the durable truncation. The delivery adapter
// implements it by listing the transcript and decoding the wire-shaped run blobs
// — which only the wire layer may decode — into the domain timeline.
type BoundaryResolver func(ctx context.Context) (transcript.Boundary, error)

// RollbackFiles executes a session rollback as one guarded operation: it claims
// the single-writer mutation slot (rejecting a rollback under an in-flight run
// as [ErrSessionBusy]) and, for a file restore, the working-tree mutation slot
// too, then resolves the boundary under those guards, restores the working tree
// to the run snapshot (files first — the atomicity guarantee for a both-rollback,
// AUX_API §4.1), and applies the durable history truncation. It returns the
// session so the delivery adapter can shape its response without re-reading it.
//
// The guards live here, not at the wire: a file restore's `git reset --hard`
// writes a working tree a sibling session sharing the cwd would race, and that
// sibling's tool writes never take the checkpoint lock, so the mutation must see
// any in-flight run on the tree (ActiveSessionWithCwd), not just this session's.
func (c *Coordinator) RollbackFiles(ctx context.Context, claims SessionClaimer, spec RollbackSpec, resolve BoundaryResolver) (session.Session, error) {
	ses, err := c.s.Session().Get(ctx, spec.SessionID)
	if err != nil {
		return session.Session{}, err
	}

	admission, err := c.ClaimMutationSlot(claims, spec.SessionID)
	if err != nil {
		return ses, err
	}
	defer admission.Release()

	var cwd string
	if spec.RestoreFiles {
		cwd = worktree.CanonicalCwd(ses.Cwd)
		if cwd == "" {
			return ses, ErrCheckpointUnavailable
		}
		treeAdmission, ok := c.ClaimWorkingTreeMutation(cwd)
		if !ok {
			return ses, fmt.Errorf("%w: working tree %q has a run admission in flight", ErrSessionBusy, cwd)
		}
		defer treeAdmission.Release()
		if busy := claims.ActiveSessionWithCwd(cwd); busy != "" {
			return ses, fmt.Errorf("%w: session %q shares this working tree and has a run in flight", ErrSessionBusy, busy)
		}
	}

	boundary, err := resolve(ctx)
	if err != nil {
		return ses, err
	}

	// A both-rollback that drops runs mutates two resources that can't share one
	// transaction — the working tree (git) and the durable history (SQLite) — so
	// log the intent before either changes (§8.5); a crash mid-operation is then
	// re-driven at boot. Single-resource rollbacks (files-only or history-only)
	// commit in one resource and need no log.
	recoverable := spec.RestoreFiles && spec.RestoreHistory && len(boundary.Dropped) > 0
	if recoverable {
		if err := c.recordMutation(ctx, execution.WorkspaceMutation{
			SessionID: spec.SessionID, Cwd: cwd, ToRunID: spec.ToRunID,
		}); err != nil {
			return ses, err
		}
	}

	// Files first: git reset is atomic, so a restore error leaves the tree
	// unchanged — clear the just-logged intent rather than let boot force-complete
	// a rollback the caller saw fail (a missing snapshot would never recover).
	if spec.RestoreFiles {
		if err := c.restore(ctx, spec.SessionID, cwd, spec.ToRunID); err != nil {
			if recoverable {
				_ = c.completeMutation(ctx, spec.SessionID)
			}
			return ses, err
		}
	}

	// The tree is restored now; a durable failure here leaves the intent logged so
	// boot recovery completes the truncation (the tree + history would otherwise
	// disagree).
	if spec.RestoreHistory && len(boundary.Dropped) > 0 {
		if err := c.Rollback(ctx, spec.SessionID, boundary); err != nil {
			return ses, err
		}
	}

	if recoverable {
		if err := c.completeMutation(ctx, spec.SessionID); err != nil {
			return ses, err
		}
	}
	return ses, nil
}

// BoundaryLookup rebuilds the durable rollback cut for a (sessionID, toRunID)
// the way the live path does — decoding the wire-shaped run blobs the
// coordinator can't. Boot recovery drives it per logged intent.
type BoundaryLookup func(ctx context.Context, sessionID, toRunID string) (transcript.Boundary, error)

// RecoverWorkspaceMutations re-drives every file rollback a crash left
// unfinished (§8.5): for each logged intent it re-restores the working tree
// (reentrant) and re-applies the durable truncation (idempotent — a rollback
// that already committed recomputes an empty boundary), then clears the intent.
// It runs at boot before the server serves, so no run contends for the session
// and the admission guards the live path needs are unnecessary. A failed
// recovery aborts startup (returned loud) rather than serving a session whose
// tree and history disagree.
func (c *Coordinator) RecoverWorkspaceMutations(ctx context.Context, lookup BoundaryLookup) error {
	if c.mutations == nil {
		return nil
	}
	pending, err := c.mutations.ListPending(ctx)
	if err != nil {
		return err
	}
	for _, m := range pending {
		if err := c.recoverRollback(ctx, m, lookup); err != nil {
			return fmt.Errorf("recover rollback for session %q: %w", m.SessionID, err)
		}
	}
	return nil
}

func (c *Coordinator) recoverRollback(ctx context.Context, m execution.WorkspaceMutation, lookup BoundaryLookup) error {
	boundary, err := lookup(ctx, m.SessionID, m.ToRunID)
	if err != nil {
		return err
	}
	if err := c.restore(ctx, m.SessionID, m.Cwd, m.ToRunID); err != nil {
		return err
	}
	if len(boundary.Dropped) > 0 {
		if err := c.Rollback(ctx, m.SessionID, boundary); err != nil {
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

// restore drives the checkpoint store, mapping a nil store (file checkpoints
// disabled) onto [ErrCheckpointUnavailable] so a build without checkpoints
// rejects file restore rather than nil-panicking.
func (c *Coordinator) restore(ctx context.Context, sessionID, cwd, runID string) error {
	if c.checkpoints == nil {
		return ErrCheckpointUnavailable
	}
	return c.checkpoints.Restore(ctx, sessionID, cwd, runID)
}
