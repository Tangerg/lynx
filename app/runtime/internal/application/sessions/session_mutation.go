package sessions

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
)

// DeleteSession atomically removes all durable session state (the atomic
// write-set), then tears down process-local parked turns and the resume gate,
// and finally drops the session's working-tree checkpoints. The open interrupts
// are read up front so the abandoned turns can be canceled after the durable
// state is gone. Checkpoint cleanup runs last, after the durable delete has
// already succeeded; all post-commit cleanup failures are returned together.
func (c *Coordinator) DeleteSession(ctx context.Context, sessionID string) error {
	admission, err := c.ClaimMutationSlot(sessionID)
	if err != nil {
		return err
	}
	defer admission.Release()

	var pending []interrupts.Pending
	return c.withGoalMutation(
		ctx,
		[]string{sessionID},
		func(ctx context.Context) error {
			if c.interrupts == nil {
				return errors.New("sessions: interrupt store is unavailable")
			}
			open, err := c.interrupts.List(ctx, sessionID)
			if err != nil {
				return err
			}
			pending = append(pending, open...)
			if c.writes == nil {
				return errors.New("sessions: write sets are unavailable")
			}
			return c.writes.ApplyDelete(ctx, DeletePlan{SessionID: sessionID})
		},
		func(ctx context.Context) error {
			// The durable cascade is gone as of here, so the signal cannot outrun it —
			// and it goes out before the process-local cleanup, whose failures are the
			// caller's to report but change nothing a client can read.
			c.publishAggregateMoved([]string{sessionID}, nil)
			var cleanupErrs []error
			for _, item := range pending {
				if err := c.cancelTurn(ctx, RunTurnBinding{
					RunID:     item.RootRunID,
					SessionID: item.SessionID,
					TurnID:    item.TurnID,
				}); err != nil {
					cleanupErrs = append(cleanupErrs, err)
				}
			}
			cleanupErrs = append(cleanupErrs, c.dropSessionResources([]string{sessionID}, "deleted")...)
			return errors.Join(cleanupErrs...)
		},
	)
}

// dropSessionResources removes the process-local resources which outlive a
// durable session write-set. Deletion and rollback share this exact post-commit
// cleanup order; callers choose the action only to preserve useful error
// context for the operator.
func (c *Coordinator) dropSessionResources(sessionIDs []string, action string) []error {
	var errs []error
	for _, sessionID := range sessionIDs {
		if c.forgetter != nil {
			c.forgetter.ForgetSession(sessionID)
		}
		if c.checkpoints != nil {
			if err := c.checkpoints.DropSession(sessionID); err != nil {
				errs = append(errs, fmt.Errorf("sessions: drop checkpoints for %s session %q: %w", action, sessionID, err))
			}
		}
		if c.sandbox != nil {
			if err := c.sandbox.Discard(sessionID); err != nil {
				errs = append(errs, fmt.Errorf("sessions: discard sandbox copy for %s session %q: %w", action, sessionID, err))
			}
		}
	}
	return errs
}

func (c *Coordinator) withGoalMutation(
	ctx context.Context,
	sessionIDs []string,
	commit func(context.Context) error,
	afterCommit func(context.Context) error,
) error {
	if c.goals == nil {
		if err := commit(ctx); err != nil {
			return err
		}
		return afterCommit(ctx)
	}
	return c.goals.WithSessionMutation(ctx, sessionIDs, commit, afterCommit)
}

// restoreSession applies a canonical archive and, when requested, derives its
// session view before releasing the mutation admission. A restoration must not
// expose a separately-read view because another mutation could otherwise
// interleave between the durable write and Delivery's response.
func (c *Coordinator) restoreSession(ctx context.Context, snapshot Snapshot, present bool) (SessionView, error) {
	normalized, err := snapshot.NormalizeForRestore()
	if err != nil {
		return SessionView{}, err
	}
	snapshot = normalized
	admission, err := c.ClaimRunSlot(ctx, snapshot.Session.ID)
	if err != nil {
		return SessionView{}, err
	}
	defer admission.Release()
	cwd, err := c.resolveSessionCwd(snapshot.Session.Cwd)
	if err != nil {
		return SessionView{}, err
	}
	snapshot.Session.Cwd = cwd
	var view SessionView
	err = c.withGoalMutation(
		ctx,
		[]string{snapshot.Session.ID},
		func(ctx context.Context) error {
			if c.writes == nil {
				return errors.New("sessions: write sets are unavailable")
			}
			return c.writes.ApplyRestore(ctx, restorePlan(snapshot))
		},
		func(ctx context.Context) error {
			// Restore replaced the whole history: any isolated working copy
			// from before the restore is stale, so discard it before exposing
			// the restored aggregate.
			c.publishAggregateMoved([]string{snapshot.Session.ID}, nil)
			var postCommitErrs []error
			if c.sandbox != nil {
				if discardErr := c.sandbox.Discard(snapshot.Session.ID); discardErr != nil {
					postCommitErrs = append(postCommitErrs, fmt.Errorf("sessions: discard stale sandbox copy on restore: %w", discardErr))
				}
			}
			if present {
				var viewErr error
				view, viewErr = c.view(ctx, snapshot.Session, SessionIdle)
				postCommitErrs = append(postCommitErrs, viewErr)
			}
			return errors.Join(postCommitErrs...)
		},
	)
	return view, err
}

// RestorePortableSession rebuilds and restores one transport-neutral archive.
// Archive decoding belongs to adapters; aggregate reconstruction and invariant
// enforcement belong here with the restore use case.
func (c *Coordinator) RestorePortableSession(ctx context.Context, portable PortableSnapshot) (SessionView, error) {
	snapshot, err := portable.CanonicalSnapshot()
	if err != nil {
		return SessionView{}, err
	}
	return c.restoreSession(ctx, snapshot, true)
}
