package runs

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/admission"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
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
		// The durable unique index rejected the INSERT, which means another writer got
		// there first. Naming that Run is the same answer the pre-admission check gives:
		// what changed is only who noticed.
		if errors.Is(err, execution.ErrSessionBusy) {
			if active, lookupErr := c.activeRunConflict(ctx, sess.ID); lookupErr == nil && active != nil {
				return StartResult{}, active
			}
			return StartResult{}, fmt.Errorf("%w: %w", ErrSessionBusy, err)
		}
		return StartResult{}, err
	}
	c.publishRunMoved(sess.ID, runID)
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
	if err := pending.Validate(); err != nil {
		return StartResult{}, fmt.Errorf("runs: invalid pending interrupt set: %w", err)
	}
	if gap := pending.ProtocolProfile.Uncovered(cmd.CallerCapabilities); !gap.IsEmpty() {
		return StartResult{}, &execution.ProfileNotCovered{RunID: cmd.RunID, Gap: gap}
	}
	answers, err := resolveResumeResponses(pending, cmd.Responses)
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
	rootContinuation, ok := pending.RootContinuation()
	if !ok {
		return StartResult{}, errors.New("runs: pending interrupt set has no root continuation")
	}
	segmentID := c.newSegmentID()
	createdAt := rootContinuation.RunCreatedAt
	pendingCopy := pending
	events, err := c.openSegment(ctx, segmentSpec{
		RunID:          cmd.RunID,
		SegmentID:      segmentID,
		SessionID:      pending.SessionID,
		Cwd:            sess.Cwd,
		TurnID:         turn.TurnID,
		ModelSelection: rootContinuation.ModelSelection,
		CreatedAt:      createdAt,
		Input:          cmd.Input,
		Pending:        &pendingCopy,
		admission:      &runAdmission,
		Activate: func(activateCtx context.Context) error {
			// The RUN's frozen kinds, not this request's: the caller has already been
			// checked to cover them, and taking the declaration here would let each
			// resume change what the next segment may park on.
			return c.turns.Resume(activateCtx, turn, answers, pending.ProtocolProfile.InterruptKinds)
		},
	})
	if err != nil {
		return StartResult{}, err
	}
	// The continuation is durably accepted, which consumed the whole open set: the
	// run is running again and nothing in this session is waiting on a person.
	for _, continuation := range pending.Continuations {
		if continuation.RunID == pending.RootRunID {
			continue
		}
		c.publishRunMoved(pending.SessionID, continuation.RunID)
	}
	c.publishWaitingMoved(pending.SessionID, pending.RootRunID)
	result := StartResult{RunID: cmd.RunID, SegmentID: segmentID, SessionID: pending.SessionID, Events: events}
	if len(cmd.Input) > 0 {
		// Named only when there is an item to name: the id is derived from the segment
		// the same way a fresh run derives it, so the client reconciles its optimistic
		// bubble by id rather than by content.
		result.UserItemID = userMessageItemID(segmentID)
	}
	return result, nil
}

// Cancel handles both live and parked runs under the same run/session admission
// rules and returns the exact terminal Run committed by the winning write-set.
// The durable abandon write-set is authoritative and commits before a parked
// turn is torn down. Process cleanup errors are returned unless the turn already
// disappeared, which is the idempotent completion race.
func (c *Coordinator) Cancel(ctx context.Context, cmd CancelCommand) (CancelResult, error) {
	if err := c.requireControlDependencies(); err != nil {
		return CancelResult{}, err
	}

	plan, entry, live, err := c.cancellationPlanFor(ctx, cmd)
	if err != nil {
		return CancelResult{}, err
	}
	if plan.target.run.Lineage().IsChild() {
		switch plan.treeState {
		case execution.Running:
			if !live || entry.handle == nil {
				return CancelResult{}, fmt.Errorf(
					"runs: running child Run %q has no live root owner",
					cmd.RunID,
				)
			}
			return c.cancelLiveChild(ctx, cmd, plan, entry.handle)
		case execution.Interrupted:
			return CancelResult{}, fmt.Errorf(
				"runs: interrupted child Run %q cancellation is not implemented",
				cmd.RunID,
			)
		default:
			return CancelResult{}, fmt.Errorf(
				"runs: child Run %q belongs to a tree in state %s",
				cmd.RunID,
				plan.treeState,
			)
		}
	}
	if !live {
		return c.cancelWithoutLiveSegment(ctx, cmd, plan.root.run)
	}
	if entry.handle == nil {
		return CancelResult{}, fmt.Errorf(
			"runs: root Run %q has a live registry entry without a handle",
			plan.root.run.ID,
		)
	}
	cleanupCtx, cancel := entry.handle.cleanupContext(ctx)
	defer cancel()
	interruptCommitted, requestErr := entry.handle.requestCancel(cleanupCtx, cmd.Reason)
	if requestErr != nil {
		if errors.Is(requestErr, ErrSessionBusy) {
			return CancelResult{}, requestErr
		}
		c.registry.MarkCancel(plan.root.run.ID, cmd.Reason)
		return CancelResult{}, errors.Join(requestErr, entry.handle.wait(cleanupCtx))
	}
	c.registry.MarkCancel(plan.root.run.ID, cmd.Reason)
	if interruptCommitted {
		// The interrupt transaction won before cancellation. Its pump owns the
		// live admission until it has published and closed the parked segment;
		// join that boundary, then apply the durable parked cancel transaction.
		if err := entry.handle.wait(cleanupCtx); err != nil {
			return CancelResult{}, err
		}
		return c.cancelKnownParkedRun(cleanupCtx, cmd, plan.root.run, plan.turn)
	}
	// The pump owns every non-parked live teardown. requestCancel has stopped its
	// stream context; joining the handle returns that single owner's complete
	// cleanup boundary without racing a second CancelTurn from this goroutine.
	if err := entry.handle.wait(cleanupCtx); err != nil {
		return CancelResult{}, err
	}
	terminal, committed := entry.handle.committedTerminalRun()
	if !committed {
		return CancelResult{}, fmt.Errorf(
			"runs: canceled live root Run %q completed without a terminal snapshot",
			plan.root.run.ID,
		)
	}
	if terminal.State != execution.Canceled {
		return CancelResult{}, fmt.Errorf(
			"%w: %q completed as %s",
			ErrRunFinished,
			plan.root.run.ID,
			terminal.State,
		)
	}
	return rootCancelResult(terminal)
}

func (c *Coordinator) cancelLiveChild(
	ctx context.Context,
	cmd CancelCommand,
	plan cancellationPlan,
	live *handle,
) (CancelResult, error) {
	attempt, err := live.beginChildCancellation(plan, cmd.Reason)
	if err != nil {
		return CancelResult{}, err
	}
	cleanupCtx, cancel := live.cleanupContext(ctx)
	defer cancel()
	if err := c.turns.CancelSubtree(
		cleanupCtx,
		plan.turn,
		plan.target.source.ProcessID,
	); err != nil {
		live.abortChildCancellation(attempt, err)
		return CancelResult{}, err
	}
	target, root, err := live.waitChildCancellation(cleanupCtx, attempt)
	if err != nil {
		return CancelResult{}, err
	}
	if target.ID != plan.target.run.ID ||
		target.State != execution.Canceled ||
		target.Outcome == nil ||
		*target.Outcome != execution.OutcomeCanceled {
		return CancelResult{}, fmt.Errorf(
			"runs: child cancellation for %q committed invalid target snapshot %q in state %s",
			plan.target.run.ID,
			target.ID,
			target.State,
		)
	}
	if root.ID != plan.root.run.ID || !root.Lineage().IsRoot() {
		return CancelResult{}, fmt.Errorf(
			"runs: child cancellation for %q returned invalid root snapshot %q",
			plan.target.run.ID,
			root.ID,
		)
	}
	return CancelResult{Run: target, RootRun: &root}, nil
}

// cancelWithoutLiveSegment resolves the small window in which a segment has
// left the process registry after the first durable read. A second durable read
// classifies a real terminal race; a still-running orphan is an invariant fault,
// never run_not_found.
func (c *Coordinator) cancelWithoutLiveSegment(ctx context.Context, cmd CancelCommand, run transcript.Run) (CancelResult, error) {
	if run.State == execution.Interrupted {
		cleanupCtx, cancel := (*handle)(nil).cleanupContext(ctx)
		defer cancel()
		return c.cancelParkedRun(cleanupCtx, cmd, run)
	}
	refreshed, found, err := c.runs.Run(ctx, cmd.RunID)
	switch {
	case err != nil:
		return CancelResult{}, err
	case !found:
		return CancelResult{}, fmt.Errorf("runs: run %q disappeared after it was resolved", cmd.RunID)
	case refreshed.State.IsTerminal():
		return CancelResult{}, fmt.Errorf("%w: %q completed as %s", ErrRunFinished, cmd.RunID, refreshed.State)
	case refreshed.State == execution.Interrupted:
		cleanupCtx, cancel := (*handle)(nil).cleanupContext(ctx)
		defer cancel()
		return c.cancelParkedRun(cleanupCtx, cmd, refreshed)
	case refreshed.State == execution.Running:
		return CancelResult{}, fmt.Errorf(
			"runs: run %q is running segment %q with no live owner",
			cmd.RunID, refreshed.ActiveSegmentID,
		)
	default:
		return CancelResult{}, fmt.Errorf("runs: run %q has unknown state %d", cmd.RunID, refreshed.State)
	}
}

// cancelParkedRun claims the Session before resolving its open interrupt. That
// admission order linearizes cancel against resume: whichever command owns the
// Session decides the one durable transition, and the loser observes busy or
// the resulting terminal state instead of misreporting run_not_found.
func (c *Coordinator) cancelParkedRun(ctx context.Context, cmd CancelCommand, run transcript.Run) (CancelResult, error) {
	releaseSession, ok := c.admission.AcquireSession(run.SessionID)
	if !ok {
		return CancelResult{}, ErrSessionBusy
	}
	defer releaseSession()

	pending, found, err := c.sessions.GetOpenInterrupt(ctx, cmd.RunID)
	if err != nil {
		return CancelResult{}, err
	}
	if !found {
		refreshed, exists, lookupErr := c.runs.Run(ctx, cmd.RunID)
		switch {
		case lookupErr != nil:
			return CancelResult{}, lookupErr
		case !exists:
			return CancelResult{}, fmt.Errorf("runs: parked run %q disappeared while its session was claimed", cmd.RunID)
		case refreshed.State.IsTerminal():
			return CancelResult{}, fmt.Errorf("%w: %q completed as %s", ErrRunFinished, cmd.RunID, refreshed.State)
		default:
			return CancelResult{}, fmt.Errorf("runs: run %q is %s but has no open interrupt", cmd.RunID, refreshed.State)
		}
	}
	if pending.SessionID != run.SessionID {
		return CancelResult{}, fmt.Errorf(
			"runs: run %q belongs to session %q but its interrupt belongs to %q",
			cmd.RunID, run.SessionID, pending.SessionID,
		)
	}
	return c.cancelClaimedParkedRun(ctx, cmd, execution.TurnRef{
		SessionID: pending.SessionID,
		TurnID:    pending.TurnID,
	})
}

// cancelKnownParkedRun is used only after the live handle proves its interrupt
// transaction committed. The handle's segment binding is therefore the exact
// turn that transaction parked; resolving it again would introduce a second,
// weaker source of identity between two halves of one command.
func (c *Coordinator) cancelKnownParkedRun(
	ctx context.Context,
	cmd CancelCommand,
	run transcript.Run,
	ref execution.TurnRef,
) (CancelResult, error) {
	if ref.SessionID != run.SessionID {
		return CancelResult{}, fmt.Errorf(
			"runs: run %q belongs to session %q but its live turn belongs to %q",
			cmd.RunID, run.SessionID, ref.SessionID,
		)
	}
	releaseSession, ok := c.admission.AcquireSession(run.SessionID)
	if !ok {
		return CancelResult{}, ErrSessionBusy
	}
	defer releaseSession()
	return c.cancelClaimedParkedRun(ctx, cmd, ref)
}

func (c *Coordinator) cancelClaimedParkedRun(ctx context.Context, cmd CancelCommand, ref execution.TurnRef) (CancelResult, error) {
	terminal, err := c.sessions.ApplyRunCancel(ctx, ref.SessionID, cmd.RunID, cmd.Reason, c.now().UTC())
	if err != nil {
		return CancelResult{}, err
	}
	// The abandon write-set publishes its own invalidation: it is the transaction that
	// ends the run and drops the interrupt, and it is reached from here and from a
	// resume that finds the park unresumable. Signaling here too would be a second
	// author for one commit.
	if err := c.turns.CancelTurn(ctx, ref); err != nil && !errors.Is(err, ErrTurnNotLive) {
		return CancelResult{}, fmt.Errorf("runs: clean up canceled parked run %q turn: %w", cmd.RunID, err)
	}
	return rootCancelResult(terminal)
}

func rootCancelResult(run transcript.Run) (CancelResult, error) {
	if run.Lineage().IsChild() {
		return CancelResult{}, fmt.Errorf("runs: canceled root result %q is a child run", run.ID)
	}
	if run.State != execution.Canceled || run.Outcome == nil || *run.Outcome != execution.OutcomeCanceled {
		return CancelResult{}, fmt.Errorf("runs: cancel committed invalid terminal run %q in state %s", run.ID, run.State)
	}
	return CancelResult{Run: run}, nil
}

// Steer addresses the segment the command names and lets the turn adapter
// recover the concrete executor handle.
//
// It resolves through the same authority a subscribe does
// ([Coordinator.addressLiveSegment]), so "this run is waiting" or "that segment
// has been replaced" is one answer with one spelling rather than two entry
// points each guessing from the live registry.
func (c *Coordinator) Steer(ctx context.Context, cmd SteerCommand) error {
	if c.turns == nil {
		return errors.New("runs: turn control is required")
	}
	live, err := c.addressLiveSegment(ctx, cmd.RunID, cmd.ExpectedSegmentID)
	if err != nil {
		return err
	}
	rec := live.record
	if err := c.turns.Steer(ctx, execution.TurnRef{SessionID: rec.SessionID, TurnID: rec.TurnID}, cmd.Input); err != nil {
		if errors.Is(err, ErrTurnNotLive) {
			// The turn ended between resolving the record and delivering: the run is
			// finishing, which is the same thing the durable record would say a moment
			// from now.
			return fmt.Errorf("%w: %w", ErrRunFinished, err)
		}
		return err
	}
	return nil
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
		// The in-process gate also guards working-tree mutations, so what it refuses is
		// not always a Run and cannot always be named.
		return admission.RunAdmission{}, ErrSessionBusy
	}
	// A Run the Session already holds is reported WITH its identity: the caller has to
	// choose between steering it, answering it and canceling it, and it cannot choose
	// without knowing which run and what state. Waiting counts — a Run parked on a
	// person is still the Session's Run.
	active, err := c.activeRunConflict(ctx, sess.ID)
	if err != nil {
		runAdmission.Release()
		return admission.RunAdmission{}, err
	}
	if active != nil {
		runAdmission.Release()
		return admission.RunAdmission{}, active
	}
	return runAdmission, nil
}

// activeRunConflict reports the Session's non-terminal Run as a conflict, or nil when
// it has none. One author, because the same conflict is reachable twice: this process
// can see the Run before admission, and the durable unique index can reject the
// INSERT after another process created one.
func (c *Coordinator) activeRunConflict(ctx context.Context, sessionID string) (error, error) {
	run, found, err := c.sessions.ActiveRun(ctx, sessionID)
	if err != nil || !found {
		return nil, err
	}
	return &ActiveRunConflict{RunID: run.ID, Status: run.State.Status()}, nil
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
	root, ok := pending.RootContinuation()
	if !ok {
		return execution.TurnRef{}, errors.Join(
			ErrRunNotFound,
			errors.New("runs: interrupt has no root continuation"),
		)
	}
	turn, err = c.turns.Rehydrate(ctx, RehydrateTurn{
		SessionID:                pending.SessionID,
		TurnID:                   pending.TurnID,
		ProcessID:                root.ProcessID,
		ModelSelection:           root.ModelSelection,
		Cwd:                      cwd,
		ChildRunAdmissionEnabled: len(pending.Continuations) > 1,
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
	if c.runs == nil {
		return errors.New("runs: run projection is required")
	}
	return nil
}
