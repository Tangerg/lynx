package runs

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/admission"
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
	if err := cmd.ValidateScheduledIdentity(); err != nil {
		return StartResult{}, err
	}
	message, media, openingUserText, err := cmd.MaterializeInput()
	if err != nil {
		return StartResult{}, err
	}
	draft := StartTurn{
		Message:        message,
		Media:          media,
		ModelSelection: cmd.ModelSelection,
		MaxBudget:      cmd.MaxBudget,
		MaxCostUSD:     cmd.MaxCostUSD,
		MaxSteps:       cmd.MaxSteps,
		Options:        cmd.Options,
		InterruptKinds: cmd.ProtocolProfile.InterruptKinds,
		GoalLeaseID:    cmd.GoalLeaseID,
	}
	if err := draft.Validate(); err != nil {
		return StartResult{}, err
	}
	if err := c.turns.ValidateStart(draft); err != nil {
		return StartResult{}, err
	}

	sess, scheduled, err := c.resolveSession(ctx, cmd.SessionID, cmd.NewSessionID, cmd.DefaultCwd, cmd.NewSessionTitle)
	if err != nil {
		return StartResult{}, err
	}
	runAdmission, err := c.claimFreshRun(ctx, sess)
	if err != nil {
		return StartResult{}, err
	}
	defer runAdmission.Release()

	draft.SessionID = sess.ID
	execCwd, isolated, err := c.executionCwd(ctx, sess)
	if err != nil {
		return StartResult{}, err
	}
	draft.Cwd = execCwd
	draft.Isolated = isolated
	turn, err := c.turns.PrepareStart(ctx, draft)
	if err != nil {
		return StartResult{}, err
	}
	if err := c.validateStartedTurn(ctx, turn, sess.ID); err != nil {
		return StartResult{}, err
	}

	runID := cmd.RunID
	if runID == "" {
		runID = c.newRunID()
	}
	segmentID := c.newSegmentID()
	createdAt := c.now().UTC()
	var sessionModel *SessionModelUpdate
	if cmd.ModelSelection.Configured() {
		sessionModel = &SessionModelUpdate{SessionID: sess.ID, Model: cmd.ModelSelection.Model()}
	}
	events, err := c.openSegment(ctx, segmentSpec{
		RunID:            runID,
		SegmentID:        segmentID,
		SessionID:        sess.ID,
		Cwd:              sess.Cwd,
		TurnID:           turn.TurnID,
		ModelSelection:   cmd.ModelSelection,
		GoalLeaseID:      cmd.GoalLeaseID,
		ScheduledSession: scheduled,
		SessionModel:     sessionModel,
		ScheduleFiring:   cmd.ScheduleFiring,
		CreatedAt:        createdAt,
		OpeningUserText:  openingUserText,
		Input:            cmd.Input,
		Limits:           execution.RunLimits{MaxSteps: cmd.MaxSteps, MaxBudgetUSD: cmd.MaxCostUSD},
		ProtocolProfile:  cmd.ProtocolProfile,
		admission:        &runAdmission,
		Activate: func(activateCtx context.Context) error {
			return c.turns.Activate(activateCtx, turn)
		},
	})
	if err != nil {
		if errors.Is(err, execution.ErrSessionBusy) {
			return StartResult{}, fmt.Errorf("%w: %w", ErrSessionBusy, err)
		}
		return StartResult{}, err
	}
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
	if gap := pending.ProtocolProfile.Uncovered(cmd.CallerCapabilities); !gap.IsEmpty() {
		return StartResult{}, fmt.Errorf("%w: run %q was created with %s", ErrProfileNotCovered, cmd.RunID, gap)
	}
	resolution, err := resolveResumeResponses(pending, cmd.Responses)
	if err != nil {
		return StartResult{}, err
	}
	sess, err := c.sessions.Get(ctx, pending.SessionID)
	if err != nil {
		return StartResult{}, err
	}
	runAdmission, ok := c.admission.AcquireRun(pending.SessionID, sess.Cwd)
	if !ok {
		return StartResult{}, fmt.Errorf("%w: session %q or working tree %q has a run or mutation in flight", ErrSessionBusy, pending.SessionID, sess.Cwd)
	}
	defer runAdmission.Release()

	// Resume inherits the copy cwd + isolation from the parked turn's Runtime
	// scope, so no execution-cwd resolution is needed here. A rehydrate (process
	// gone) of an isolated run is refused as lost — see prepareTurn — because the
	// sandbox copy died with the process.
	turn, err := c.prepareTurn(ctx, pending, sess.Cwd, sess.Isolated)
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
		RunID:          cmd.RunID,
		SegmentID:      segmentID,
		SessionID:      pending.SessionID,
		Cwd:            sess.Cwd,
		TurnID:         turn.TurnID,
		ModelSelection: pending.ModelSelection,
		CreatedAt:      createdAt,
		Pending:        &pendingCopy,
		admission:      &runAdmission,
		Activate: func(activateCtx context.Context) error {
			// The RUN's frozen kinds, not this request's: the caller has already been
			// checked to cover them, and taking the declaration here would let each
			// resume change what the next segment may park on.
			return c.turns.Resume(activateCtx, turn, resolution, pending.ProtocolProfile.InterruptKinds)
		},
	})
	if err != nil {
		return StartResult{}, err
	}
	return StartResult{RunID: cmd.RunID, SegmentID: segmentID, SessionID: pending.SessionID, Events: events}, nil
}

// Cancel handles both live and parked runs under the same run/session admission
// rules. The durable abandon write-set is authoritative and commits before a
// parked turn is torn down. Process cleanup errors are returned unless the turn
// already disappeared, which is the idempotent completion race.
func (c *Coordinator) Cancel(ctx context.Context, cmd CancelCommand) error {
	if err := c.requireControlDependencies(); err != nil {
		return err
	}
	entry, live := c.registry.Get(cmd.RunID)
	cleanupCtx, cancel := entry.handle.cleanupContext(ctx)
	defer cancel()
	if live {
		ref := execution.TurnRef{
			SessionID: entry.record.SessionID,
			TurnID:    entry.record.TurnID,
		}
		interruptCommitted, requestErr := entry.handle.requestCancel(cleanupCtx, cmd.Reason)
		c.registry.MarkCancel(cmd.RunID, cmd.Reason)
		if requestErr != nil {
			return errors.Join(requestErr, entry.handle.wait(cleanupCtx))
		}
		if interruptCommitted {
			// The interrupt transaction won before cancellation. Its pump owns the
			// live admission until it has published and closed the parked segment;
			// join that boundary, then apply the known durable cancel directly.
			if err := entry.handle.wait(cleanupCtx); err != nil {
				return err
			}
			return c.cancelParkedBinding(cleanupCtx, cmd, ref)
		}
		// The pump owns every non-parked live teardown. requestCancel has stopped
		// its stream context; joining the handle returns that single owner's
		// cleanup result without racing a second CancelTurn from this goroutine.
		return entry.handle.wait(cleanupCtx)
	}
	return c.cancelParkedRun(cleanupCtx, cmd)
}

// cancelParkedRun applies the durable cancel write-set to a run parked on an open
// interrupt. Live cancellation never probes this store to infer a race outcome:
// the handle's interrupt boundary reports that outcome directly.
func (c *Coordinator) cancelParkedRun(ctx context.Context, cmd CancelCommand) error {
	pending, found, err := c.sessions.GetOpenInterrupt(ctx, cmd.RunID)
	if err != nil {
		return err
	}
	if !found {
		return ErrRunNotFound
	}
	return c.cancelParkedBinding(ctx, cmd, execution.TurnRef{
		SessionID: pending.SessionID,
		TurnID:    pending.TurnID,
	})
}

func (c *Coordinator) cancelParkedBinding(ctx context.Context, cmd CancelCommand, ref execution.TurnRef) error {
	releaseSession, ok := c.admission.AcquireSession(ref.SessionID)
	if !ok {
		return ErrSessionBusy
	}
	defer releaseSession()
	if err := c.sessions.ApplyRunCancel(ctx, ref.SessionID, cmd.RunID, cmd.Reason, c.now().UTC()); err != nil {
		return err
	}
	if err := c.turns.CancelTurn(ctx, ref); err != nil && !errors.Is(err, ErrTurnNotLive) {
		return fmt.Errorf("runs: clean up canceled parked run %q turn: %w", cmd.RunID, err)
	}
	return nil
}

// Steer addresses a live run by its application record and lets the turn
// adapter recover the concrete executor handle.
func (c *Coordinator) Steer(ctx context.Context, cmd SteerCommand) error {
	if c.turns == nil {
		return errors.New("runs: turn control is required")
	}
	rec, ok := c.liveRecord(cmd.RunID)
	if !ok {
		return ErrRunNotFound
	}
	if err := c.turns.Steer(ctx, execution.TurnRef{SessionID: rec.SessionID, TurnID: rec.TurnID}, cmd.Message); err != nil {
		if errors.Is(err, ErrTurnNotLive) {
			return fmt.Errorf("%w: %w", ErrRunNotFound, err)
		}
		return err
	}
	return nil
}

func (c *Coordinator) liveRecord(runID string) (Record, bool) {
	e, ok := c.registry.Get(runID)
	if !ok {
		return Record{}, false
	}
	return e.record, true
}

func (c *Coordinator) resolveSession(ctx context.Context, id, newID, defaultCwd, title string) (session.Session, *session.Session, error) {
	if newID != "" {
		sess, err := c.sessions.PrepareScheduled(ctx, newID, title, defaultCwd)
		if err != nil {
			return session.Session{}, nil, err
		}
		return sess, &sess, nil
	}
	if id == "" {
		sess, err := c.sessions.Create(ctx, title, defaultCwd)
		return sess, nil, err
	}
	sess, err := c.sessions.Get(ctx, id)
	return sess, nil, err
}

func (c *Coordinator) claimFreshRun(ctx context.Context, sess session.Session) (admission.RunAdmission, error) {
	runAdmission, ok := c.admission.AcquireRun(sess.ID, sess.Cwd)
	if !ok {
		return admission.RunAdmission{}, ErrSessionBusy
	}
	open, err := c.sessions.ListOpenInterrupts(ctx, sess.ID)
	if err != nil {
		runAdmission.Release()
		return admission.RunAdmission{}, err
	}
	if len(open) > 0 {
		runAdmission.Release()
		return admission.RunAdmission{}, ErrSessionBusy
	}
	return runAdmission, nil
}

// executionCwd resolves where a session's turn tools operate: the sandbox copy
// for an isolated session (created on first use), else the project directory.
// It fails closed when isolation is requested but unavailable — an isolated run
// must never fall back to the real tree.
func (c *Coordinator) executionCwd(ctx context.Context, sess session.Session) (cwd string, isolated bool, err error) {
	if !sess.Isolated {
		return sess.Cwd, false, nil
	}
	if c.isolation == nil {
		return "", false, fmt.Errorf("%w: isolation is not configured", ErrIsolationUnavailable)
	}
	copyDir, err := c.isolation.Workspace(ctx, sess.ID, sess.Cwd)
	if err != nil {
		return "", false, fmt.Errorf("%w: %w", ErrIsolationUnavailable, err)
	}
	return copyDir, true, nil
}

func (c *Coordinator) prepareTurn(ctx context.Context, pending interrupts.Pending, cwd string, isolated bool) (execution.TurnRef, error) {
	turn, err := c.turns.Prepare(ctx, execution.TurnRef{SessionID: pending.SessionID, TurnID: pending.TurnID})
	if err == nil {
		if err := turn.ValidateFor(pending.SessionID); err != nil {
			return execution.TurnRef{}, err
		}
		return turn, nil
	}
	if errors.Is(err, ErrParkClaimed) {
		return execution.TurnRef{}, ErrInterruptNotOpen
	}
	if !errors.Is(err, ErrTurnNotLive) {
		return execution.TurnRef{}, err
	}
	// The parked turn is not live in this process, so its executor died — for an
	// isolated run that means its sandbox copy, which lives only in this process's
	// Isolator, died with it. Rehydrating would rebuild the turn against the
	// project directory (the only cwd we still have), running the resumed model
	// and its memory extraction on the REAL tree — the exact pollution isolation
	// exists to prevent. Fail closed: the run's world is gone, so it is lost, not
	// resumable. Reusing ErrTurnStateLost routes it through the same durable
	// lost-run cleanup as a missing process snapshot.
	if isolated {
		return execution.TurnRef{}, fmt.Errorf("%w: an isolated run cannot resume after its sandbox process ended", ErrTurnStateLost)
	}
	if pending.ProcessID == "" {
		return execution.TurnRef{}, errors.Join(ErrRunNotFound, errors.New("runs: interrupt has no recorded process id"))
	}
	turn, err = c.turns.Rehydrate(ctx, RehydrateTurn{
		SessionID:      pending.SessionID,
		TurnID:         pending.TurnID,
		ProcessID:      pending.ProcessID,
		ModelSelection: pending.ModelSelection,
		Cwd:            cwd,
	})
	if err != nil {
		return execution.TurnRef{}, errors.Join(ErrRunNotFound, err)
	}
	if err := turn.ValidateFor(pending.SessionID); err != nil {
		return execution.TurnRef{}, err
	}
	return turn, nil
}

func (c *Coordinator) validateStartedTurn(ctx context.Context, ref execution.TurnRef, sessionID string) error {
	if err := ref.ValidateFor(sessionID); err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runCleanupTimeout)
		defer cancel()
		if cleanupErr := c.turns.CancelTurn(cleanupCtx, ref); cleanupErr != nil {
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
	case c.admission == nil:
		return errors.New("runs: admission gate is required")
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
	if c.admission == nil {
		return errors.New("runs: admission gate is required")
	}
	return nil
}
