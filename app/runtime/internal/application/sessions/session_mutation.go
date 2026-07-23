package sessions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// DeleteSession atomically removes all durable session state (the atomic
// write-set), then tears down process-local parked turns and the resume gate,
// and finally drops the session's working-tree checkpoints. The open interrupts
// are read up front so the abandoned turns can be canceled after the durable
// state is gone. Checkpoint cleanup runs last, after the durable delete has
// already succeeded; all post-commit cleanup failures are returned together.
func (c *Coordinator) DeleteSession(ctx context.Context, sessionID string) error {
	admissions, sessionIDs, err := c.claimDeleteTree(ctx, sessionID)
	if err != nil {
		return err
	}
	defer func() {
		for i := len(admissions) - 1; i >= 0; i-- {
			admissions[i].Release()
		}
	}()

	var pending []interrupts.Pending
	if err := c.withGoalMutation(ctx, sessionIDs, func(ctx context.Context) error {
		for _, id := range sessionIDs {
			if c.interrupts == nil {
				return errors.New("sessions: interrupt store is unavailable")
			}
			open, err := c.interrupts.List(ctx, id)
			if err != nil {
				return err
			}
			pending = append(pending, open...)
		}
		if c.writes == nil {
			return errors.New("sessions: write sets are unavailable")
		}
		return c.writes.ApplyDelete(ctx, DeletePlan{SessionIDs: sessionIDs})
	}); err != nil {
		return err
	}
	var cleanupErrs []error
	for _, item := range pending {
		if err := c.cancelTurn(ctx, RunTurnBinding{
			RunID:     item.RunID,
			SessionID: item.SessionID,
			TurnID:    item.TurnID,
		}); err != nil {
			cleanupErrs = append(cleanupErrs, err)
		}
	}
	for _, id := range sessionIDs {
		if c.forgetter != nil {
			c.forgetter.ForgetSession(id)
		}
		if c.checkpoints != nil {
			if err := c.checkpoints.DropSession(id); err != nil {
				cleanupErrs = append(cleanupErrs, fmt.Errorf("sessions: drop checkpoints for deleted session %q: %w", id, err))
			}
		}
		if c.sandbox != nil {
			if err := c.sandbox.Discard(id); err != nil {
				cleanupErrs = append(cleanupErrs, fmt.Errorf("sessions: discard sandbox copy for deleted session %q: %w", id, err))
			}
		}
	}
	return errors.Join(cleanupErrs...)
}

func (c *Coordinator) claimDeleteTree(ctx context.Context, sessionID string) ([]RunAdmission, []string, error) {
	root, err := c.ClaimMutationSlot(sessionID)
	if err != nil {
		return nil, nil, err
	}
	admissions := []RunAdmission{root}
	var sessionIDs []string
	seen := map[string]struct{}{sessionID: {}}

	var visit func(string, bool) error
	visit = func(parentID string, ownedSubtree bool) error {
		if c.sessions == nil {
			return errors.New("sessions: session store is unavailable")
		}
		children, err := c.sessions.Children(ctx, parentID)
		if err != nil {
			return err
		}
		for _, child := range children {
			if !ownedSubtree && child.Kind != session.KindSubtask {
				continue
			}
			if _, exists := seen[child.ID]; exists {
				return fmt.Errorf("sessions: delete tree contains duplicate or cyclic session %q", child.ID)
			}
			seen[child.ID] = struct{}{}
			admission, err := c.ClaimMutationSlot(child.ID)
			if err != nil {
				return err
			}
			admissions = append(admissions, admission)
			if err := visit(child.ID, true); err != nil {
				return err
			}
			sessionIDs = append(sessionIDs, child.ID)
		}
		return nil
	}
	if err := visit(sessionID, false); err != nil {
		for i := len(admissions) - 1; i >= 0; i-- {
			admissions[i].Release()
		}
		return nil, nil, err
	}
	return admissions, append(sessionIDs, sessionID), nil
}

func (c *Coordinator) claimMutationSlots(sessionIDs []string) ([]RunAdmission, error) {
	admissions := make([]RunAdmission, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		admission, err := c.ClaimMutationSlot(sessionID)
		if err != nil {
			releaseAdmissions(admissions)
			return nil, err
		}
		admissions = append(admissions, admission)
	}
	return admissions, nil
}

func (c *Coordinator) prepareRollbackSessions(ctx context.Context, sessionID string, boundary transcript.Boundary) ([]string, []RunAdmission, error) {
	if len(boundary.Dropped) == 0 {
		return nil, nil, nil
	}
	sessionIDs, err := c.subtaskSessionsAfter(ctx, sessionID, boundary.BoundaryTime)
	if err != nil {
		return nil, nil, err
	}
	admissions, err := c.claimMutationSlots(sessionIDs)
	if err != nil {
		return nil, nil, err
	}
	return sessionIDs, admissions, nil
}

func releaseAdmissions(admissions []RunAdmission) {
	for i := len(admissions) - 1; i >= 0; i-- {
		admissions[i].Release()
	}
}

func (c *Coordinator) withGoalMutation(ctx context.Context, sessionIDs []string, apply func(context.Context) error) error {
	if c.goals == nil {
		return apply(ctx)
	}
	return c.goals.WithSessionMutation(ctx, sessionIDs, apply)
}

// RestoreSession recreates a session under its ORIGINAL id from one validated
// canonical snapshot and replaces its whole history as one atomic write-set (§8.1): it
// upserts the session record, clears the old interrupts / admission rows /
// transcript / chat log, then re-seeds the messages, runs, and items. Without
// the single transaction a mid-sequence failure (after the destructive clear)
// would leave the session row live but its history half-destroyed.
//
// Restore owns the Session admission boundary (including rejection of an open
// interrupt), then resolves Cwd exactly as Create and Update do before
// committing the aggregate. It validates regardless of caller, so correctness
// never depends on a particular protocol adapter.
func (c *Coordinator) RestoreSession(ctx context.Context, snapshot Snapshot) error {
	normalized, err := snapshot.NormalizeForRestore()
	if err != nil {
		return err
	}
	snapshot = normalized
	admission, err := c.ClaimRunSlot(ctx, snapshot.Session.ID)
	if err != nil {
		return err
	}
	defer admission.Release()
	cwd, err := c.resolveSessionCwd(snapshot.Session.Cwd)
	if err != nil {
		return err
	}
	snapshot.Session.Cwd = cwd
	if err := c.withGoalMutation(ctx, []string{snapshot.Session.ID}, func(ctx context.Context) error {
		if c.writes == nil {
			return errors.New("sessions: write sets are unavailable")
		}
		return c.writes.ApplyRestore(ctx, snapshot)
	}); err != nil {
		return err
	}
	// Restore replaced the whole history: any isolated working copy from before
	// the restore is stale, so drop it post-commit and let the next isolated run
	// rebuild a fresh copy from the restored project. Best-effort cleanup.
	if c.sandbox != nil {
		if err := c.sandbox.Discard(snapshot.Session.ID); err != nil {
			return fmt.Errorf("sessions: discard stale sandbox copy on restore: %w", err)
		}
	}
	return nil
}

// RestorePortableSession rebuilds and restores one transport-neutral archive.
// Archive decoding belongs to adapters; aggregate reconstruction and invariant
// enforcement belong here with the restore use case.
func (c *Coordinator) RestorePortableSession(ctx context.Context, portable PortableSnapshot) error {
	snapshot, err := portable.CanonicalSnapshot()
	if err != nil {
		return err
	}
	return c.RestoreSession(ctx, snapshot)
}

// subtaskSessionsAfter resolves the internal subtask subtrees a rollback must
// delete. User-created forks share ParentID but have an empty Kind and remain
// independent conversations; only KindSubtask roots are attributed to runs.
// IDs are post-order so descendants are deleted before their parent.
func (c *Coordinator) subtaskSessionsAfter(ctx context.Context, parentID string, boundary time.Time) ([]string, error) {
	if c.sessions == nil {
		return nil, errors.New("sessions: session store is unavailable")
	}
	children, err := c.sessions.Children(ctx, parentID)
	if err != nil {
		return nil, err
	}
	var out []string
	seen := map[string]struct{}{parentID: {}}
	for _, child := range children {
		if child.Kind != session.KindSubtask {
			continue
		}
		if !boundary.IsZero() && child.StartedAt.Before(boundary) {
			continue
		}
		if err := c.appendSessionSubtree(ctx, child.ID, seen, &out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (c *Coordinator) appendSessionSubtree(ctx context.Context, sessionID string, seen map[string]struct{}, out *[]string) error {
	if _, exists := seen[sessionID]; exists {
		return fmt.Errorf("sessions: rollback tree contains duplicate or cyclic session %q", sessionID)
	}
	seen[sessionID] = struct{}{}
	if c.sessions == nil {
		return errors.New("sessions: session store is unavailable")
	}
	children, err := c.sessions.Children(ctx, sessionID)
	if err != nil {
		return err
	}
	for _, child := range children {
		if err := c.appendSessionSubtree(ctx, child.ID, seen, out); err != nil {
			return err
		}
	}
	*out = append(*out, sessionID)
	return nil
}
