package sessions

import (
	"context"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// DeleteSession atomically removes all durable session state (the atomic
// write-set), then tears down process-local parked turns and the resume gate,
// and finally drops the session's working-tree checkpoints. The open interrupts
// are read up front so the abandoned turns can be canceled after the durable
// state is gone. The checkpoint drop is best-effort cleanup (a shadow-git
// concern) run last, after the durable delete has already succeeded.
func (c *Coordinator) DeleteSession(ctx context.Context, claims SessionClaimer, sessionID string) error {
	admissions, sessionIDs, err := c.claimDeleteTree(ctx, claims, sessionID)
	if err != nil {
		return err
	}
	defer func() {
		for i := len(admissions) - 1; i >= 0; i-- {
			admissions[i].Release()
		}
	}()

	var pending []interrupts.Pending
	for _, id := range sessionIDs {
		open, err := c.s.Interrupts().List(ctx, id)
		if err != nil {
			return err
		}
		pending = append(pending, open...)
	}
	if err := c.s.ApplyDelete(ctx, DeletePlan{SessionIDs: sessionIDs}); err != nil {
		return err
	}
	for _, item := range pending {
		c.cancelTurn(ctx, RunTurnBinding{
			RunID:     item.RunID,
			SessionID: item.SessionID,
			TurnID:    item.TurnID,
		})
	}
	for _, id := range sessionIDs {
		c.s.ForgetSession(id)
		if c.checkpoints != nil {
			_ = c.checkpoints.DropSession(id)
		}
	}
	return nil
}

func (c *Coordinator) claimDeleteTree(ctx context.Context, claims SessionClaimer, sessionID string) ([]RunAdmission, []string, error) {
	root, err := c.ClaimMutationSlot(claims, sessionID)
	if err != nil {
		return nil, nil, err
	}
	admissions := []RunAdmission{root}
	var sessionIDs []string
	seen := map[string]struct{}{sessionID: {}}

	var visit func(string, bool) error
	visit = func(parentID string, ownedSubtree bool) error {
		children, err := c.s.Session().Children(ctx, parentID)
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
			admission, err := c.ClaimMutationSlot(claims, child.ID)
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

func claimMutationSlots(claims SessionClaimer, sessionIDs []string) ([]RunAdmission, error) {
	if claims == nil {
		return nil, nil
	}
	admissions := make([]RunAdmission, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		if !claims.ClaimSession(sessionID) {
			releaseAdmissions(admissions)
			return nil, ErrSessionBusy
		}
		admissions = append(admissions, heldAdmission(claims, sessionID))
	}
	return admissions, nil
}

func (c *Coordinator) prepareRollbackSessions(ctx context.Context, claims SessionClaimer, sessionID string, boundary transcript.Boundary) ([]string, []RunAdmission, error) {
	if len(boundary.Dropped) == 0 {
		return nil, nil, nil
	}
	sessionIDs, err := c.subtaskSessionsAfter(ctx, sessionID, boundary.BoundaryTime)
	if err != nil {
		return nil, nil, err
	}
	admissions, err := claimMutationSlots(claims, sessionIDs)
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

// RestoreSession recreates a session under its ORIGINAL id from a decoded
// artifact and replaces its whole history as one atomic write-set (§8.1): it
// upserts the session record, clears the old interrupts / admission rows /
// transcript / chat log, then re-seeds the messages, runs, and items. Without
// the single transaction a mid-sequence failure (after the destructive clear)
// would leave the session row live but its history half-destroyed.
//
// The caller decodes the wire artifact into these domain values. Restore owns
// the Session admission boundary (including rejection of an open interrupt),
// then resolves Cwd exactly as Create and Update do before committing the
// decoded aggregate.
func (c *Coordinator) RestoreSession(ctx context.Context, claims SessionClaimer, ses session.Session, msgs []chat.Message, runs []transcript.Run, items []transcript.Item) error {
	admission, err := c.ClaimRunSlot(ctx, claims, ses.ID)
	if err != nil {
		return err
	}
	defer admission.Release()
	cwd, err := c.resolveSessionCwd(ses.Cwd)
	if err != nil {
		return err
	}
	ses.Cwd = cwd
	return c.s.ApplyRestore(ctx, RestorePlan{
		Session:  ses,
		Messages: msgs,
		Runs:     runs,
		Items:    items,
	})
}

// subtaskSessionsAfter resolves the internal subtask subtrees a rollback must
// delete. User-created forks share ParentID but have an empty Kind and remain
// independent conversations; only KindSubtask roots are attributed to runs.
// IDs are post-order so descendants are deleted before their parent.
func (c *Coordinator) subtaskSessionsAfter(ctx context.Context, parentID string, boundary time.Time) ([]string, error) {
	children, err := c.s.Session().Children(ctx, parentID)
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
	children, err := c.s.Session().Children(ctx, sessionID)
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
