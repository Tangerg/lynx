package sessions

import (
	"context"
	"errors"
	"fmt"

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

	// Files first: if the working tree can't be restored, return before the
	// history changes so a both-rollback leaves the log untouched.
	if spec.RestoreFiles {
		if err := c.restore(ctx, spec.SessionID, cwd, spec.ToRunID); err != nil {
			return ses, err
		}
	}

	if spec.RestoreHistory && len(boundary.Dropped) > 0 {
		if err := c.Rollback(ctx, spec.SessionID, boundary); err != nil {
			return ses, err
		}
	}
	return ses, nil
}

// restore drives the checkpoint restorer, mapping a nil restorer (file
// checkpoints disabled) onto [ErrCheckpointUnavailable] so a build without
// checkpoints rejects file restore rather than nil-panicking.
func (c *Coordinator) restore(ctx context.Context, sessionID, cwd, runID string) error {
	if c.restorer == nil {
		return ErrCheckpointUnavailable
	}
	return c.restorer.Restore(ctx, sessionID, cwd, runID)
}
