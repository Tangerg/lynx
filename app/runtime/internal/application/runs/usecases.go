package runs

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// Start validates and resolves the session, claims the session and working
// tree, starts the executor turn, mints run identity, and hands the prepared
// segment to the package's existing lifecycle supervisor.
func (c *Coordinator) Start(ctx context.Context, cmd StartCommand) (StartResult, error) {
	if err := c.requireUseCaseDependencies(); err != nil {
		return StartResult{}, err
	}
	draft := StartTurn{
		Message:        cmd.Message,
		Media:          cmd.Media,
		Provider:       cmd.Provider,
		Model:          cmd.Model,
		MaxBudget:      cmd.MaxBudget,
		MaxCostUSD:     cmd.MaxCostUSD,
		MaxSteps:       cmd.MaxSteps,
		Options:        cmd.Options,
		InterruptKinds: cmd.InterruptKinds,
	}
	if err := draft.Validate(); err != nil {
		return StartResult{}, err
	}
	if err := c.turns.ValidateStart(draft); err != nil {
		return StartResult{}, err
	}

	sess, err := c.resolveSession(ctx, cmd.SessionID, cmd.DefaultCwd, cmd.NewSessionTitle)
	if err != nil {
		return StartResult{}, err
	}
	if err := c.claimFreshRun(ctx, sess.ID); err != nil {
		return StartResult{}, err
	}
	defer c.ReleaseSession(sess.ID)

	releaseTree, ok := c.sessions.AcquireWorkingTreeRun(sess.Cwd)
	if !ok {
		return StartResult{}, fmt.Errorf("%w: working tree %q has a mutation in flight", ErrSessionBusy, sess.Cwd)
	}
	releaseTreeOnReturn := true
	defer func() {
		if releaseTreeOnReturn {
			releaseTree()
		}
	}()

	if cmd.Model != "" {
		if err := c.sessions.SetModel(ctx, sess.ID, cmd.Model); err != nil {
			return StartResult{}, err
		}
	}
	draft.SessionID = sess.ID
	draft.Cwd = sess.Cwd
	turn, err := c.turns.Start(ctx, draft)
	if err != nil {
		return StartResult{}, err
	}
	if err := c.validateStartedTurn(ctx, turn, sess.ID); err != nil {
		return StartResult{}, err
	}

	runID, segmentID := c.newRunID(), c.newSegmentID()
	createdAt := c.now().UTC()
	events, err := c.openSegment(ctx, segmentSpec{
		RunID:           runID,
		SegmentID:       segmentID,
		SessionID:       sess.ID,
		Cwd:             sess.Cwd,
		TurnID:          turn.TurnID,
		Provider:        cmd.Provider,
		Model:           cmd.Model,
		CreatedAt:       createdAt,
		OpeningUserText: cmd.OpeningUserText,
		Input:           cmd.Input,
	})
	if err != nil {
		if errors.Is(err, execution.ErrSessionBusy) {
			return StartResult{}, fmt.Errorf("%w: %w", ErrSessionBusy, err)
		}
		return StartResult{}, err
	}
	releaseTree()
	releaseTreeOnReturn = false
	return StartResult{
		RunID: runID, SegmentID: segmentID, SessionID: sess.ID,
		UserItemID: userMessageItemID(segmentID), Events: events,
	}, nil
}

// Resume claims the parked run's session, prepares or rehydrates its turn,
// attaches and durably accepts a continuation segment, and only then activates
// the user's resolution.
func (c *Coordinator) Resume(ctx context.Context, cmd ResumeCommand) (StartResult, error) {
	if err := c.requireUseCaseDependencies(); err != nil {
		return StartResult{}, err
	}
	pending, found, err := c.sessions.GetOpenInterrupt(ctx, cmd.RunID)
	if err != nil {
		return StartResult{}, err
	}
	if !found {
		return StartResult{}, ErrInterruptNotOpen
	}
	resolution, err := resolveResumeResponses(pending, cmd.Responses)
	if err != nil {
		return StartResult{}, err
	}
	if !c.ClaimSession(pending.SessionID) {
		return StartResult{}, fmt.Errorf("%w: session %q has a run in flight", ErrSessionBusy, pending.SessionID)
	}
	defer c.ReleaseSession(pending.SessionID)

	sess, err := c.sessions.Get(ctx, pending.SessionID)
	if err != nil {
		return StartResult{}, err
	}
	releaseTree, ok := c.sessions.AcquireWorkingTreeRun(sess.Cwd)
	if !ok {
		return StartResult{}, fmt.Errorf("%w: working tree %q has a mutation in flight", ErrSessionBusy, sess.Cwd)
	}
	releaseTreeOnReturn := true
	defer func() {
		if releaseTreeOnReturn {
			releaseTree()
		}
	}()

	turn, err := c.prepareTurn(ctx, pending, sess.Cwd)
	if err != nil {
		if errors.Is(err, ErrTurnStateLost) {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runCleanupTimeout)
			cleanupErr := c.sessions.ApplyRunLost(cleanupCtx, pending.SessionID, cmd.RunID, c.now().UTC())
			cancel()
			if cleanupErr != nil {
				return StartResult{}, errors.Join(err, fmt.Errorf("runs: recover lost run %q: %w", cmd.RunID, cleanupErr))
			}
			return StartResult{}, fmt.Errorf("%w: %w", ErrRunNotFound, err)
		}
		return StartResult{}, err
	}
	segmentID := c.newSegmentID()
	createdAt := pending.RunCreatedAt
	pendingCopy := pending
	events, err := c.openSegment(ctx, segmentSpec{
		RunID:     cmd.RunID,
		SegmentID: segmentID,
		SessionID: pending.SessionID,
		Cwd:       sess.Cwd,
		TurnID:    turn.TurnID,
		Provider:  pending.Provider,
		Model:     pending.Model,
		CreatedAt: createdAt,
		Pending:   &pendingCopy,
		Activate: func(activateCtx context.Context) error {
			return c.turns.Resume(activateCtx, turn, resolution, cmd.InterruptKinds)
		},
	})
	if err != nil {
		return StartResult{}, err
	}
	releaseTree()
	releaseTreeOnReturn = false
	return StartResult{RunID: cmd.RunID, SegmentID: segmentID, SessionID: pending.SessionID, Events: events}, nil
}

// Cancel handles both live and parked runs under the same run/session admission
// rules. Executor cancellation is best-effort; the durable abandon write-set is
// authoritative and must succeed.
func (c *Coordinator) Cancel(ctx context.Context, cmd CancelCommand) error {
	if err := c.requireControlDependencies(); err != nil {
		return err
	}
	binding, cleanupCtx, cancel, live := c.BeginCancel(ctx, cmd.RunID, cmd.Reason)
	if live {
		defer cancel()
		_ = c.turns.Cancel(cleanupCtx, TurnRef(binding))
		return nil
	}

	pending, found, err := c.sessions.GetOpenInterrupt(ctx, cmd.RunID)
	if err != nil {
		return err
	}
	if !found {
		return ErrRunNotFound
	}
	if !c.ClaimSession(pending.SessionID) {
		return ErrSessionBusy
	}
	defer c.ReleaseSession(pending.SessionID)
	cleanupCtx, cancel = context.WithTimeout(context.WithoutCancel(ctx), runCleanupTimeout)
	if err := c.sessions.ApplyRunCancel(cleanupCtx, pending.SessionID, cmd.RunID, cmd.Reason, c.now().UTC()); err != nil {
		cancel()
		return err
	}
	cancel()
	turnCtx, cancelTurn := context.WithTimeout(context.WithoutCancel(ctx), runCleanupTimeout)
	defer cancelTurn()
	_ = c.turns.Cancel(turnCtx, TurnRef{SessionID: pending.SessionID, TurnID: pending.TurnID})
	return nil
}

// Steer addresses a live run by its application record and lets the turn
// adapter recover the concrete executor handle.
func (c *Coordinator) Steer(ctx context.Context, cmd SteerCommand) error {
	if c.turns == nil {
		return errors.New("runs: turn control is required")
	}
	rec, ok := c.LiveRun(cmd.RunID)
	if !ok {
		return ErrRunNotFound
	}
	if err := c.turns.Steer(ctx, TurnRef{SessionID: rec.SessionID, TurnID: rec.TurnID}, cmd.Message); err != nil {
		if errors.Is(err, ErrTurnNotLive) {
			return fmt.Errorf("%w: %w", ErrRunNotFound, err)
		}
		return err
	}
	return nil
}

func (c *Coordinator) resolveSession(ctx context.Context, id, defaultCwd, title string) (session.Session, error) {
	if id == "" {
		return c.sessions.Create(ctx, title, defaultCwd)
	}
	return c.sessions.Get(ctx, id)
}

func (c *Coordinator) claimFreshRun(ctx context.Context, sessionID string) error {
	if !c.ClaimSession(sessionID) {
		return ErrSessionBusy
	}
	open, err := c.sessions.ListOpenInterrupts(ctx, sessionID)
	if err != nil {
		c.ReleaseSession(sessionID)
		return err
	}
	if len(open) > 0 {
		c.ReleaseSession(sessionID)
		return ErrSessionBusy
	}
	return nil
}

func (c *Coordinator) prepareTurn(ctx context.Context, pending interrupts.Pending, cwd string) (TurnRef, error) {
	turn, err := c.turns.Prepare(ctx, TurnRef{SessionID: pending.SessionID, TurnID: pending.TurnID})
	if err == nil {
		if err := turn.ValidateFor(pending.SessionID); err != nil {
			return TurnRef{}, err
		}
		return turn, nil
	}
	if errors.Is(err, ErrParkClaimed) {
		return TurnRef{}, ErrInterruptNotOpen
	}
	if !errors.Is(err, ErrTurnNotLive) {
		return TurnRef{}, err
	}
	if pending.ProcessID == "" {
		return TurnRef{}, errors.Join(ErrRunNotFound, errors.New("runs: interrupt has no recorded process id"))
	}
	turn, err = c.turns.Rehydrate(ctx, RehydrateTurn{
		SessionID: pending.SessionID,
		TurnID:    pending.TurnID,
		ProcessID: pending.ProcessID,
		Provider:  pending.Provider,
		Model:     pending.Model,
		Cwd:       cwd,
	})
	if err != nil {
		return TurnRef{}, errors.Join(ErrRunNotFound, err)
	}
	if err := turn.ValidateFor(pending.SessionID); err != nil {
		return TurnRef{}, err
	}
	return turn, nil
}

func (c *Coordinator) validateStartedTurn(ctx context.Context, ref TurnRef, sessionID string) error {
	if err := ref.ValidateFor(sessionID); err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runCleanupTimeout)
		defer cancel()
		if cleanupErr := c.turns.Cancel(cleanupCtx, ref); cleanupErr != nil {
			return errors.Join(err, fmt.Errorf("runs: cancel invalid started turn: %w", cleanupErr))
		}
		return err
	}
	return nil
}

func (c *Coordinator) requireUseCaseDependencies() error {
	switch {
	case c.executor == nil:
		return errors.New("runs: segment executor is required")
	case c.turns == nil:
		return errors.New("runs: turn control is required")
	case c.sessions == nil:
		return errors.New("runs: session lifecycle is required")
	case c.effects == nil:
		return errors.New("runs: effects are required")
	case c.now == nil:
		return errors.New("runs: clock is required")
	case c.newRunID == nil:
		return errors.New("runs: run id generator is required")
	case c.newSegmentID == nil:
		return errors.New("runs: segment id generator is required")
	default:
		return nil
	}
}

func (c *Coordinator) requireControlDependencies() error {
	if c.turns == nil {
		return errors.New("runs: turn control is required")
	}
	if c.sessions == nil {
		return errors.New("runs: session lifecycle is required")
	}
	return nil
}
