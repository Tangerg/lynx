package runs

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/admission"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// Start validates and resolves the session, claims the session and working
// tree, starts execution, mints Run identity, and hands the prepared
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
	draft := StartExecution{
		Message:                  message,
		Media:                    media,
		ModelSelection:           cmd.ModelSelection,
		Limits:                   cmd.Limits,
		Options:                  cmd.Options,
		InterruptKinds:           cmd.Capabilities.InterruptKinds,
		ChildRunAdmissionEnabled: cmd.Capabilities.ChildRuns,
		GoalLeaseID:              cmd.GoalLeaseID,
	}
	if err := draft.Validate(); err != nil {
		return StartResult{}, err
	}
	if err := c.control.ValidateStart(draft); err != nil {
		return StartResult{}, err
	}

	sess, scheduled, err := c.resolveSession(ctx, cmd.SessionID, cmd.NewSessionID, cmd.DefaultWorkspacePath, cmd.NewSessionTitle)
	if err != nil {
		return StartResult{}, err
	}
	runAdmission, err := c.claimFreshRun(ctx, sess)
	if err != nil {
		return StartResult{}, err
	}
	defer runAdmission.Release()

	draft.SessionID = sess.ID
	execCWD, isolated, err := c.executionCWD(ctx, sess)
	if err != nil {
		return StartResult{}, err
	}
	draft.CWD = execCWD
	draft.WorkspaceCWD = sess.CWD
	draft.Isolated = isolated
	ref, err := c.control.PrepareStart(ctx, draft)
	if err != nil {
		return StartResult{}, err
	}
	if err := c.validatePreparedExecution(ctx, ref, sess.ID); err != nil {
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
		CWD:              sess.CWD,
		ExecutorID:       ref.ExecutorID,
		ModelSelection:   cmd.ModelSelection,
		GoalLeaseID:      cmd.GoalLeaseID,
		ScheduledSession: scheduled,
		SessionModel:     sessionModel,
		ScheduleFiring:   cmd.ScheduleFiring,
		CreatedAt:        createdAt,
		OpeningUserText:  openingUserText,
		Input:            cmd.Input,
		Limits:           cmd.Limits,
		Capabilities:     cmd.Capabilities,
		admission:        &runAdmission,
		Activate: func(activateCtx context.Context) error {
			return c.control.Activate(activateCtx, ref)
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

func (c *Coordinator) resolveSession(ctx context.Context, id, newID, defaultWorkspacePath, title string) (session.Session, *session.Session, error) {
	if newID != "" {
		sess, err := c.sessions.PrepareScheduled(ctx, newID, title, defaultWorkspacePath)
		if err != nil {
			return session.Session{}, nil, err
		}
		return sess, &sess, nil
	}
	if id == "" {
		sess, err := c.sessions.Create(ctx, title, defaultWorkspacePath)
		return sess, nil, err
	}
	sess, err := c.sessions.Get(ctx, id)
	return sess, nil, err
}

func (c *Coordinator) claimFreshRun(ctx context.Context, sess session.Session) (admission.RunAdmission, error) {
	runAdmission, ok := c.admission.AcquireRun(sess.ID, sess.CWD)
	if !ok {
		// The in-process gate also guards working-tree mutations, so what it refuses is
		// not always a Run and cannot always be named.
		return admission.RunAdmission{}, ErrRunAdmissionBusy
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

// executionCWD resolves where a Session's tools operate: the sandbox copy
// for an isolated session (created on first use), else the project directory.
// It fails closed when isolation is requested but unavailable — an isolated run
// must never fall back to the real tree.
func (c *Coordinator) executionCWD(ctx context.Context, sess session.Session) (cwd string, isolated bool, err error) {
	if !sess.Isolated {
		return sess.CWD, false, nil
	}
	if c.isolation == nil {
		return "", false, fmt.Errorf("%w: isolation is not configured", ErrIsolationUnavailable)
	}
	copyDir, err := c.isolation.Workspace(ctx, sess.ID, sess.CWD)
	if err != nil {
		return "", false, fmt.Errorf("%w: %w", ErrIsolationUnavailable, err)
	}
	return copyDir, true, nil
}

func (c *Coordinator) validatePreparedExecution(ctx context.Context, ref execution.ExecutorRef, sessionID string) error {
	if err := ref.ValidateFor(sessionID); err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runCleanupTimeout)
		defer cancel()
		if cleanupErr := c.control.CancelExecution(cleanupCtx, ref); cleanupErr != nil {
			return errors.Join(err, fmt.Errorf("runs: cancel invalid started executor: %w", cleanupErr))
		}
		return err
	}
	return nil
}
