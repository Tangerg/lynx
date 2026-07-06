// Package lifecycle owns the cross-domain atomic write-sets behind a few
// session/run lifecycle use-cases — rollback truncation, the session-delete
// cascade, the import/restore sequence, and the subagent subtree purge. Each
// spans several domain stores (the session row, the transcript, the chat history
// log, open interrupts) and several commit as ONE transaction via RunInTx, so a
// mid-sequence failure leaves no half-mutated session.
//
// These are use-case orchestration, not protocol adaptation: keeping them here
// (driven by the protocol adapter, which still owns wire decode and streaming
// registry concerns) holds the "thin delivery" line and lets the write-sets be
// tested without standing up the wire. The adapter lifts wire blobs into domain
// values; the Coordinator decides and executes the multi-domain mutation
// atomically.
package lifecycle

import (
	"context"
	"errors"
	"time"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

// Stores is the consumer-defined surface the Coordinator drives — the runtime's
// session-scoped stores plus the chat history log, the process-local resume
// gate (ForgetSession), and the transactional seam (RunInTx). The composition
// root's runtime bundle satisfies it; defined here so the Coordinator depends
// only on what it calls, not the whole runtime.
type Stores interface {
	Session() session.Service
	Transcript() transcript.Store
	Interrupts() interrupts.Store
	// ReadHistory returns the chat history log for a session.
	ReadHistory(ctx context.Context, sessionID string) ([]chat.Message, error)
	// TruncateMessages clamps a session's chat history log to keepN messages
	// (keepN=0 clears it).
	TruncateMessages(ctx context.Context, sessionID string, keepN int) error
	// SeedHistory replaces a session's chat history log with msgs.
	SeedHistory(ctx context.Context, sessionID string, msgs []chat.Message) error
	// ForgetSession releases the turn service's process-local state for a
	// session that is being removed (the SessionStart gate) — see turn.Service.
	ForgetSession(sessionID string)
	// RunInTx runs fn inside one storage transaction; the store calls the
	// closure makes join it through the context.
	RunInTx(ctx context.Context, fn func(context.Context) error) error
}

// Coordinator executes session/run lifecycle write-sets across the domain
// stores. Stateless beyond its Stores handle; safe to share.
type Coordinator struct {
	s Stores
}

// ErrRunNotFound reports that a lifecycle operation targeted no live or parked run.
var ErrRunNotFound = errors.New("lifecycle: run not found")

// ErrInterruptNotOpen reports that an interrupt resume/cancel target is no
// longer open.
var ErrInterruptNotOpen = errors.New("lifecycle: interrupt not open")

// ErrSessionBusy reports that a session already has an active or parked run.
var ErrSessionBusy = errors.New("lifecycle: session busy")

// New returns a Coordinator over s.
func New(s Stores) *Coordinator { return &Coordinator{s: s} }

// SessionClaimer is the run-admission slot used to enforce one writer per
// session across active runs and start/resume races.
type SessionClaimer interface {
	ClaimSession(sessionID string) bool
	ReleaseSession(sessionID string)
}

// TurnCanceler is the turn-service slice needed to abandon a run.
type TurnCanceler interface {
	Cancel(context.Context, turn.TurnHandle) error
}

// RunAdmission is a held single-writer slot. Release must be called exactly
// once by the caller after the run segment is registered or admission fails.
type RunAdmission struct {
	SessionID string
	release   func()
}

// Release drops the held single-writer slot.
func (a RunAdmission) Release() {
	if a.release != nil {
		a.release()
	}
}

// TurnResumer is the turn-service slice needed to continue an interrupt.
type TurnResumer interface {
	Resume(context.Context, turn.TurnHandle, interrupts.Resolution) error
	Rehydrate(context.Context, turn.RehydrateRequest) (turn.TurnHandle, error)
}

// RunTurn binds a protocol run id to the turn handle that owns its process.
type RunTurn struct {
	RunID     string
	SessionID string
	TurnID    string
}

// ResumedInterrupt is the claimed interrupt plus the turn handle its
// continuation should stream from.
type ResumedInterrupt struct {
	Pending interrupts.Pending
	Handle  turn.TurnHandle
}

// RollbackBoundary is the resolved, domain-level rollback split.
type RollbackBoundary struct {
	KeepMark     int
	Dropped      []transcript.RunNode
	BoundaryTime time.Time
	DropRunIDs   []string
}

// ResolveRollbackBoundary computes the root-run inclusive-keep boundary for a
// rollback. The protocol adapter owns wire decoding; once it has lifted stored
// RunRefs into [transcript.RunNode], the decision is a lifecycle use-case, not
// protocol adaptation.
func ResolveRollbackBoundary(nodes []transcript.RunNode, toRunID string) (RollbackBoundary, error) {
	b, err := transcript.BoundaryAt(nodes, toRunID, true)
	if err != nil {
		return RollbackBoundary{}, err
	}
	dropIDs := make([]string, len(b.Dropped))
	for i, rec := range b.Dropped {
		dropIDs[i] = rec.ID
	}
	return RollbackBoundary{
		KeepMark:     b.KeepMark,
		Dropped:      b.Dropped,
		BoundaryTime: b.BoundaryTime,
		DropRunIDs:   dropIDs,
	}, nil
}

// Rollback truncates the chat history log to keepMark and drops each run's
// durable items + record + dangling interrupt as ONE transaction, then cancels
// any in-process parked turns that were abandoned and purges the subagent child
// sessions spawned at/after purgeAfter. A keepMark < 0 (unknown watermark —
// chain terminal still in-flight / pre-watermark) leaves the log untouched
// rather than guessing at a boundary that was never recorded.
func (c *Coordinator) Rollback(ctx context.Context, turns TurnCanceler, sessionID string, keepMark int, dropRunIDs []string, purgeAfter time.Time) error {
	parked, err := c.parkedTurns(ctx, dropRunIDs)
	if err != nil {
		return err
	}
	if err := c.s.RunInTx(ctx, func(ctx context.Context) error {
		if keepMark >= 0 {
			if err := c.s.TruncateMessages(ctx, sessionID, keepMark); err != nil {
				return err
			}
		}
		// Surfaced (not swallowed): after the truncate above commits, a failed
		// DeleteRun would otherwise leave a run record past the boundary whose
		// messages are already gone — an orphan inconsistent with the log.
		for _, runID := range dropRunIDs {
			if err := c.s.Transcript().DeleteRun(ctx, sessionID, runID); err != nil {
				return err
			}
			if err := c.s.Interrupts().Delete(ctx, runID); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	for _, r := range parked {
		c.cancelTurn(ctx, turns, r)
	}
	c.purgeChildrenAfter(ctx, sessionID, purgeAfter)
	return nil
}

// RollbackResolved executes a previously resolved rollback boundary.
func (c *Coordinator) RollbackResolved(ctx context.Context, turns TurnCanceler, sessionID string, b RollbackBoundary) error {
	if len(b.Dropped) == 0 {
		return nil
	}
	return c.Rollback(ctx, turns, sessionID, b.KeepMark, b.DropRunIDs, b.BoundaryTime)
}

// ClaimRunSlot reserves a session's single-writer slot for a fresh run and
// rejects sessions already parked on an open interrupt.
func (c *Coordinator) ClaimRunSlot(ctx context.Context, claims SessionClaimer, sessionID string) (RunAdmission, error) {
	if !claims.ClaimSession(sessionID) {
		return RunAdmission{}, ErrSessionBusy
	}
	admission := RunAdmission{
		SessionID: sessionID,
		release:   func() { claims.ReleaseSession(sessionID) },
	}
	open, err := c.s.Interrupts().List(ctx, sessionID)
	if err != nil {
		admission.Release()
		return RunAdmission{}, err
	}
	if len(open) > 0 {
		admission.Release()
		return RunAdmission{}, ErrSessionBusy
	}
	return admission, nil
}

// ClaimMutationSlot reserves a session's single-writer slot for a destructive
// session mutation. Unlike [Coordinator.ClaimRunSlot], it does not reject open
// interrupts: rollback/delete/import decide what to do with parked runs inside
// their own lifecycle write-set.
func (c *Coordinator) ClaimMutationSlot(claims SessionClaimer, sessionID string) (RunAdmission, error) {
	if !claims.ClaimSession(sessionID) {
		return RunAdmission{}, ErrSessionBusy
	}
	return RunAdmission{
		SessionID: sessionID,
		release:   func() { claims.ReleaseSession(sessionID) },
	}, nil
}

// ClaimResumeSlot peeks an open interrupt to find its session, then reserves
// that session's single-writer slot before the interrupt is consumed.
func (c *Coordinator) ClaimResumeSlot(ctx context.Context, claims SessionClaimer, parentRunID string) (interrupts.Pending, RunAdmission, error) {
	pending, found, err := c.s.Interrupts().Get(ctx, parentRunID)
	if err != nil {
		return interrupts.Pending{}, RunAdmission{}, err
	}
	if !found {
		return interrupts.Pending{}, RunAdmission{}, ErrInterruptNotOpen
	}
	if !claims.ClaimSession(pending.SessionID) {
		return pending, RunAdmission{}, ErrSessionBusy
	}
	return pending, RunAdmission{
		SessionID: pending.SessionID,
		release:   func() { claims.ReleaseSession(pending.SessionID) },
	}, nil
}

// CancelParkedRun abandons a run that has already left the live run stream and
// is discoverable only through its open interrupt record.
func (c *Coordinator) CancelParkedRun(ctx context.Context, turns TurnCanceler, runID string) error {
	pending, found, err := c.s.Interrupts().Get(ctx, runID)
	if err != nil {
		return err
	}
	if !found {
		return ErrRunNotFound
	}
	return c.CancelRunTurn(ctx, turns, RunTurn{
		RunID:     runID,
		SessionID: pending.SessionID,
		TurnID:    pending.TurnID,
	})
}

// CancelRunTurn tears down the turn before dropping the durable interrupt
// record. The turn cancel is best-effort: after a backend restart the durable
// interrupt may outlive the in-memory turn, and abandoning the run still means
// removing the resumable record.
func (c *Coordinator) CancelRunTurn(ctx context.Context, turns TurnCanceler, r RunTurn) error {
	c.cancelTurn(ctx, turns, r)
	return c.s.Interrupts().Delete(ctx, r.RunID)
}

// ResumeClaimedInterrupt consumes an open interrupt and resumes its parked
// turn. If the live turn disappeared after a backend restart, it rebuilds the
// process from the durable interrupt snapshot before returning the handle.
func (c *Coordinator) ResumeClaimedInterrupt(ctx context.Context, turns TurnResumer, parentRunID string, resolution interrupts.Resolution) (ResumedInterrupt, error) {
	pending, ok, err := c.s.Interrupts().Consume(ctx, parentRunID)
	if err != nil {
		return ResumedInterrupt{}, err
	}
	if !ok {
		return ResumedInterrupt{}, ErrInterruptNotOpen
	}

	handle := turn.TurnHandle{SessionID: pending.SessionID, TurnID: pending.TurnID}
	if err := turns.Resume(ctx, handle, resolution); err != nil {
		if errors.Is(err, turn.ErrParkClaimed) {
			return ResumedInterrupt{}, ErrInterruptNotOpen
		}
		if !errors.Is(err, turn.ErrTurnNotFound) {
			return ResumedInterrupt{}, err
		}
		handle, err = rehydratePendingTurn(ctx, turns, pending, resolution.Approved)
		if err != nil {
			return ResumedInterrupt{}, ErrRunNotFound
		}
	}

	return ResumedInterrupt{Pending: pending, Handle: handle}, nil
}

func rehydratePendingTurn(ctx context.Context, turns TurnResumer, pending interrupts.Pending, approved bool) (turn.TurnHandle, error) {
	if pending.ProcessID == "" {
		return turn.TurnHandle{}, errors.New("lifecycle: interrupt has no recorded process id")
	}
	return turns.Rehydrate(ctx, turn.RehydrateRequest{
		SessionID: pending.SessionID,
		ProcessID: pending.ProcessID,
		Approved:  approved,
		Provider:  pending.Provider,
		Model:     pending.Model,
	})
}

// ForkSpec describes where a session fork should branch. Runs are the timeline
// nodes for ParentID; empty FromRunID means copy the whole conversation.
type ForkSpec struct {
	ParentID  string
	FromRunID string
	Runs      []transcript.RunNode
	Title     string
}

// ResolveForkHistoryPrefix applies the fork boundary to a parent history. Fork
// accepts continuation runs (requireRoot=false) and an unknown watermark falls
// back to a full-history copy, matching the existing snapshot semantics.
func ResolveForkHistoryPrefix(msgs []chat.Message, nodes []transcript.RunNode, fromRunID string) ([]chat.Message, error) {
	if fromRunID == "" {
		return msgs, nil
	}
	b, err := transcript.BoundaryAt(nodes, fromRunID, false)
	if err != nil {
		return nil, err
	}
	if b.KeepMark >= 0 && b.KeepMark < len(msgs) {
		return msgs[:b.KeepMark], nil
	}
	return msgs, nil
}

// Fork creates a child session, seeds it with the resolved parent history
// prefix, and renames it as ONE transaction. The protocol adapter owns only
// wire decoding; the boundary semantics and chat history prefix live here.
func (c *Coordinator) Fork(ctx context.Context, spec ForkSpec) (session.Session, error) {
	msgs, err := c.s.ReadHistory(ctx, spec.ParentID)
	if err != nil {
		return session.Session{}, err
	}
	msgs, err = ResolveForkHistoryPrefix(msgs, spec.Runs, spec.FromRunID)
	if err != nil {
		return session.Session{}, err
	}

	var child session.Session
	if err := c.s.RunInTx(ctx, func(ctx context.Context) error {
		ch, err := c.s.Session().Fork(ctx, spec.ParentID, "")
		if err != nil {
			return err
		}
		if err := c.s.SeedHistory(ctx, ch.ID, msgs); err != nil {
			return err
		}
		if spec.Title != "" {
			if err := c.s.Session().Rename(ctx, ch.ID, spec.Title); err != nil {
				return err
			}
			ch.Title = spec.Title
		}
		child = ch
		return nil
	}); err != nil {
		return session.Session{}, err
	}
	return child, nil
}

// DeleteSession removes the session row (authoritative) then best-effort
// cascades the session-scoped storage: durable history, chat history, parked
// turns/open interrupts, and the process-local resume gate. The cascade runs
// AFTER the authoritative delete so a partial cascade leaves harmless orphans,
// never a half-deleted session. File checkpoints (shadow git) are NOT dropped
// here — that is the adapter's workspace concern, not a storage write-set.
func (c *Coordinator) DeleteSession(ctx context.Context, turns TurnCanceler, sessionID string) error {
	if err := c.s.Session().Delete(ctx, sessionID); err != nil {
		return err
	}
	_ = c.s.Transcript().DeleteSession(ctx, sessionID) // history runs + items
	_ = c.s.TruncateMessages(ctx, sessionID, 0)        // chat history messages
	c.cancelParkedInterrupts(ctx, turns, sessionID)    // live parked turns + durable interrupts
	c.s.ForgetSession(sessionID)                       // process-local SessionStart gate
	return nil
}

// RestoreSession recreates a session under its ORIGINAL id from a decoded
// artifact: it upserts the session record, clears old open interrupts, replaces
// any existing history (drop old items/runs + clear the chat log), re-seeds the
// chat messages, and re-persists the runs + items — all in one transaction.
// Without the transaction a mid-sequence failure (after the destructive
// delete/truncate) would leave the session row live but its history
// half-destroyed.
//
// The caller decodes the wire artifact into these domain values; this method
// only commits them.
func (c *Coordinator) RestoreSession(ctx context.Context, ses session.Session, msgs []chat.Message, runs []transcript.Run, items []transcript.Item) error {
	return c.s.RunInTx(ctx, func(ctx context.Context) error {
		if err := c.s.Session().Restore(ctx, ses); err != nil {
			return err
		}
		if err := c.deleteInterrupts(ctx, ses.ID); err != nil {
			return err
		}
		if err := c.s.Transcript().DeleteSession(ctx, ses.ID); err != nil {
			return err
		}
		if err := c.s.TruncateMessages(ctx, ses.ID, 0); err != nil {
			return err
		}
		if err := c.s.SeedHistory(ctx, ses.ID, msgs); err != nil {
			return err
		}
		for _, r := range runs {
			if err := c.s.Transcript().PutRun(ctx, r); err != nil {
				return err
			}
		}
		for _, it := range items {
			if err := c.s.Transcript().AppendItem(ctx, it); err != nil {
				return err
			}
		}
		return nil
	})
}

// PurgeSubtree deletes a session and its whole descendant subtree depth-first:
// chat history messages, durable history (items + runs), the session row, and
// the process-local resume gate. Best-effort — a partial failure still removes
// the leaves it reached. Used to drop a failed-fork orphan and (via
// purgeChildrenAfter) the subagent children a rollback discards.
func (c *Coordinator) PurgeSubtree(ctx context.Context, sessionID string) {
	if children, err := c.s.Session().Children(ctx, sessionID); err == nil {
		for _, child := range children {
			c.PurgeSubtree(ctx, child.ID)
		}
	}
	_ = c.s.TruncateMessages(ctx, sessionID, 0)
	_ = c.s.Transcript().DeleteSession(ctx, sessionID)
	c.dropInterrupts(ctx, sessionID)
	_ = c.s.Session().Delete(ctx, sessionID)
	c.s.ForgetSession(sessionID)
}

// purgeChildrenAfter purges the subagent child sessions of parentID spawned
// at/after boundary (a zero boundary purges all children — the drop-all
// rollback). Attribution is by spawn time: a subtask of a kept run started
// before the boundary, one of a dropped run at/after it. Exact because a
// session runs its turns sequentially and rollback requires it idle, so run
// windows don't overlap.
func (c *Coordinator) purgeChildrenAfter(ctx context.Context, parentID string, boundary time.Time) {
	children, err := c.s.Session().Children(ctx, parentID)
	if err != nil {
		return
	}
	for _, child := range children {
		if !boundary.IsZero() && child.StartedAt.Before(boundary) {
			continue
		}
		c.PurgeSubtree(ctx, child.ID)
	}
}

// dropInterrupts removes every open-interrupt record for a session. Best-effort:
// a failed list or delete leaves a resumable record that a later pass can clear.
func (c *Coordinator) dropInterrupts(ctx context.Context, sessionID string) {
	_ = c.deleteInterrupts(ctx, sessionID)
}

func (c *Coordinator) cancelParkedInterrupts(ctx context.Context, turns TurnCanceler, sessionID string) {
	pending, err := c.s.Interrupts().List(ctx, sessionID)
	if err != nil {
		return
	}
	for _, p := range pending {
		_ = c.CancelRunTurn(ctx, turns, RunTurn{
			RunID:     p.ParentRunID,
			SessionID: p.SessionID,
			TurnID:    p.TurnID,
		})
	}
}

func (c *Coordinator) parkedTurns(ctx context.Context, runIDs []string) ([]RunTurn, error) {
	out := make([]RunTurn, 0)
	for _, runID := range runIDs {
		pending, found, err := c.s.Interrupts().Get(ctx, runID)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		out = append(out, RunTurn{
			RunID:     pending.ParentRunID,
			SessionID: pending.SessionID,
			TurnID:    pending.TurnID,
		})
	}
	return out, nil
}

func (c *Coordinator) cancelTurn(ctx context.Context, turns TurnCanceler, r RunTurn) {
	if turns != nil {
		_ = turns.Cancel(ctx, turn.TurnHandle{SessionID: r.SessionID, TurnID: r.TurnID})
	}
}

func (c *Coordinator) deleteInterrupts(ctx context.Context, sessionID string) error {
	pending, err := c.s.Interrupts().List(ctx, sessionID)
	if err != nil {
		return err
	}
	for _, p := range pending {
		if err := c.s.Interrupts().Delete(ctx, p.ParentRunID); err != nil {
			return err
		}
	}
	return nil
}
