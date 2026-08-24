package sessions

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// DeleteSession atomically removes all durable session state (the atomic
// write-set), then tears down process-local parked executions and the resume gate,
// and finally drops the session's working-tree checkpoints. The open interrupts
// are read up front so the abandoned executions can be canceled after the durable
// state is gone. Checkpoint cleanup runs last, after the durable delete has
// already succeeded; all post-commit cleanup failures are returned together.
func (c *Coordinator) DeleteSession(ctx context.Context, sessionID string) error {
	admission, err := c.ClaimSessionMutation(sessionID)
	if err != nil {
		return err
	}
	defer admission.Release()

	var pending []runs.Pending
	return c.withGoalMutation(
		ctx,
		[]string{sessionID},
		func(commitCtx context.Context) error {
			open, err := c.interrupts.List(commitCtx, sessionID)
			if err != nil {
				return err
			}
			pending = append(pending, open...)
			return c.writes.ApplyDelete(commitCtx, DeletePlan{SessionID: sessionID})
		},
		func(ctx context.Context) error {
			// The durable cascade is gone as of here, so the signal cannot outrun it —
			// and it goes out before the process-local cleanup, whose failures are the
			// caller's to report but change nothing a client can read.
			c.publishAggregateMoved([]string{sessionID}, nil)
			var cleanupErrs []error
			for _, item := range pending {
				if err := c.releaseExecution(ctx, RunExecutionBinding{
					RunID:      item.RootRunID,
					SessionID:  item.SessionID,
					ExecutorID: item.ExecutorID,
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
		c.forgetter.ForgetSession(sessionID)
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
// interleave between the durable write and the returned result.
func (c *Coordinator) restoreSession(ctx context.Context, snapshot Snapshot, present bool) (View, error) {
	normalized, err := snapshot.NormalizeForRestore()
	if err != nil {
		return View{}, err
	}
	snapshot = normalized
	sessionID := snapshot.Session.ID()
	admission, err := c.ClaimIdleSession(ctx, sessionID)
	if err != nil {
		return View{}, err
	}
	defer admission.Release()
	workspace, err := c.resolveSessionWorkspace(snapshot.Session.Workspace().Path())
	if err != nil {
		return View{}, err
	}
	snapshot.Session, err = snapshot.Session.InstallRestoredWorkspace(workspace)
	if err != nil {
		return View{}, err
	}
	sessionReplacement, err := c.prepareSessionRestore(ctx, snapshot.Session)
	if err != nil {
		return View{}, err
	}
	planReplacement, err := c.prepareRestoredPlanReplacement(ctx, sessionID, snapshot.Plan)
	if err != nil {
		return View{}, err
	}
	committedSession := sessionReplacement.State()
	var view View
	err = c.withGoalMutation(
		ctx,
		[]string{sessionID},
		func(ctx context.Context) error {
			return c.writes.ApplyRestore(ctx, restorePlan(snapshot, sessionReplacement, planReplacement))
		},
		func(context.Context) error {
			// Restore replaced the whole history: any isolated working copy
			// from before the restore is stale, so discard it before exposing
			// the restored aggregate.
			c.publishAggregateMoved([]string{sessionID}, nil)
			var postCommitErrs []error
			if c.sandbox != nil {
				if discardErr := c.sandbox.Discard(sessionID); discardErr != nil {
					postCommitErrs = append(postCommitErrs, fmt.Errorf("sessions: discard stale sandbox copy on restore: %w", discardErr))
				}
			}
			if present {
				var viewErr error
				view, viewErr = c.view(committedSession, ActivityIdle)
				postCommitErrs = append(postCommitErrs, viewErr)
			}
			return errors.Join(postCommitErrs...)
		},
	)
	return view, err
}

func (c *Coordinator) prepareSessionRestore(
	ctx context.Context,
	restored session.Session,
) (SessionReplacement, error) {
	current, err := c.sessions.Get(ctx, restored.ID())
	if errors.Is(err, session.ErrNotFound) {
		return InitialSessionReplacement(restored)
	}
	if err != nil {
		return SessionReplacement{}, err
	}
	next, err := current.ReplaceWithRestore(restored, c.now())
	if err != nil {
		return SessionReplacement{}, err
	}
	return NextSessionReplacement(current, next)
}

// RestorePortableSession rebuilds and restores one transport-neutral archive.
// Boundary codecs decode the archive; aggregate reconstruction and invariant
// enforcement belong here with the restore use case.
func (c *Coordinator) RestorePortableSession(ctx context.Context, portable PortableSnapshot) (View, error) {
	snapshot, err := portable.CanonicalSnapshot()
	if err != nil {
		return View{}, err
	}
	return c.restoreSession(ctx, snapshot, true)
}
