package runs

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/sessionadmission"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	corechat "github.com/Tangerg/lynx/core/chat"
)

// Start validates and resolves the Session, claims the Session and working
// tree, stages execution, and commits the Run opening. That durable opening is
// the command's acceptance point; executor activation continues behind the
// package's lifecycle supervisor and cannot retain the accepted response.
func (c *Coordinator) Start(ctx context.Context, cmd StartCommand) (result StartResult, err error) {
	if err := cmd.ValidateScheduledIdentity(); err != nil {
		return StartResult{}, err
	}
	if err := cmd.ModelSelection.Validate(); err != nil {
		return StartResult{}, fmt.Errorf("runs: model selection: %w", err)
	}
	if !cmd.ModelSelection.Configured() {
		cmd.ModelSelection = c.defaultModelSelection
	}
	if err := cmd.ModelSelection.Validate(); err != nil {
		return StartResult{}, fmt.Errorf("runs: default model selection: %w", err)
	}
	message, media, openingUserText, err := cmd.MaterializeInput()
	if err != nil {
		return StartResult{}, err
	}
	draft := RootExecutionStart{
		Message:                  message,
		Media:                    media,
		ModelSelection:           cmd.ModelSelection,
		Limits:                   cmd.Limits,
		Options:                  cmd.Options,
		InterruptKinds:           cmd.Capabilities.InterruptKinds,
		ChildRunAdmissionEnabled: cmd.Capabilities.ChildRuns,
		GoalIncarnationID:        cmd.GoalIncarnationID,
	}
	currentMessage, err := MaterializeUserMessage(cmd.Input)
	if err != nil {
		return StartResult{}, err
	}
	draft.WorkingContext = []corechat.Message{currentMessage}
	if err := draft.Validate(); err != nil {
		return StartResult{}, err
	}
	if err := c.rootStarts.ValidateRootStart(draft); err != nil {
		return StartResult{}, err
	}

	sess, initialSession, err := c.resolveSession(
		ctx, cmd.SessionID, cmd.NewSessionID, cmd.DefaultWorkspacePath,
		cmd.NewSessionTitle, cmd.ModelSelection.Model(),
	)
	if err != nil {
		return StartResult{}, err
	}
	runAdmission, err := c.claimFreshRun(ctx, sess)
	if err != nil {
		return StartResult{}, err
	}
	defer runAdmission.Release()

	draft.SessionID = sess.ID()
	workingContext, err := c.conversation.Read(ctx, sess.ID())
	if err != nil {
		return StartResult{}, fmt.Errorf("runs: read conversation for session %q: %w", sess.ID(), err)
	}
	workingContext = append(workingContext, currentMessage.Clone())
	draft.WorkingContext = workingContext
	execCWD, isolated, err := c.executionCWD(ctx, sess)
	if err != nil {
		return StartResult{}, err
	}
	draft.CWD = execCWD
	draft.WorkspaceCWD = sess.CWD()
	draft.Isolated = isolated
	draft.WorkingContext, err = c.workingContexts.ComposeWorkingContext(ctx, WorkingContextInput{
		SessionID:  sess.ID(),
		CWD:        execCWD,
		PromptText: message,
		Seed:       draft.WorkingContext,
	})
	if err != nil {
		return StartResult{}, fmt.Errorf("runs: compose working context: %w", err)
	}
	ref, err := c.rootStarts.StageRoot(ctx, draft)
	if err != nil {
		return StartResult{}, err
	}
	staged := c.segments.ownStagedExecution(ref)
	defer func() {
		err = staged.abandon(ctx, err, "staged root execution")
		if err != nil {
			result = StartResult{}
		}
	}()
	if err := staged.validateFor(sess.ID()); err != nil {
		return StartResult{}, err
	}

	runID := cmd.RunID
	if runID == "" {
		runID = c.newRunID()
	}
	segmentID := c.newSegmentID()
	createdAt := c.publications.nowUTC()
	modelOnlyInput := cmd.GoalIncarnationID != ""
	var sessionReplacement *SessionReplacement
	if initialSession == nil && cmd.ModelSelection.Configured() {
		model := cmd.ModelSelection.Model()
		next, changed, err := sess.Apply(session.Patch{Model: &model}, createdAt)
		if err != nil {
			return StartResult{}, fmt.Errorf("runs: prepare Session model replacement: %w", err)
		}
		if changed {
			sessionReplacement = &SessionReplacement{
				ExpectedRevision: sess.Revision(), State: next,
			}
		}
	}
	// A fresh Segment owns rejection as soon as openSegment is entered. Until
	// this exact hand-off, every preparation failure remains Start's responsibility.
	ref = staged.transfer()
	events, err := c.openSegment(ctx, segmentSpec{
		RunID:              runID,
		SegmentID:          segmentID,
		SessionID:          sess.ID(),
		CWD:                sess.CWD(),
		ExecutorID:         ref.ExecutorID,
		ModelSelection:     cmd.ModelSelection,
		GoalIncarnationID:  cmd.GoalIncarnationID,
		InitialSession:     initialSession,
		SessionReplacement: sessionReplacement,
		ScheduleFiring:     cmd.ScheduleFiring,
		CreatedAt:          createdAt,
		OpeningUserText:    openingUserText,
		Input:              cmd.Input,
		ModelOnlyInput:     modelOnlyInput,
		Limits:             cmd.Limits,
		Capabilities:       cmd.Capabilities,
		admission:          &runAdmission,
		DetachActivation:   true,
		BeginExecution: func(beginCtx context.Context) error {
			return c.rootStarts.BeginRoot(beginCtx, ref)
		},
	})
	if err != nil {
		// The durable unique index rejected the INSERT, which means another writer got
		// there first. Naming that Run is the same answer the pre-admission check gives:
		// what changed is only who noticed.
		if errors.Is(err, run.ErrSessionBusy) {
			if active, lookupErr := c.activeRunConflict(ctx, sess.ID()); lookupErr == nil && active != nil {
				return StartResult{}, active
			}
			return StartResult{}, fmt.Errorf("%w: %w", ErrSessionBusy, err)
		}
		return StartResult{}, err
	}
	c.publications.publishRunMoved(sess.ID(), runID)
	userItemID := userMessageItemID(segmentID)
	if modelOnlyInput {
		userItemID = ""
	}
	return StartResult{
		RunID: runID, SegmentID: segmentID, SessionID: sess.ID(),
		UserItemID: userItemID, Events: events,
	}, nil
}

func (c *Coordinator) resolveSession(
	ctx context.Context,
	id, newID, defaultWorkspacePath, title, model string,
) (session.Session, *session.Session, error) {
	if newID != "" {
		return c.sessionCreator.PrepareScheduled(ctx, newID, title, defaultWorkspacePath, model)
	}
	if id == "" {
		sess, err := c.sessionCreator.Create(ctx, title, defaultWorkspacePath)
		return sess, nil, err
	}
	sess, err := c.sessionReader.Get(ctx, id)
	return sess, nil, err
}

func (c *Coordinator) claimFreshRun(ctx context.Context, sess session.Session) (sessionadmission.RunAdmission, error) {
	runAdmission, ok := c.admission.AcquireRun(sess.ID(), sess.CWD())
	if !ok {
		// The in-process gate also guards working-tree mutations, so what it refuses is
		// not always a Run and cannot always be named.
		return sessionadmission.RunAdmission{}, ErrRunAdmissionBusy
	}
	// A Run the Session already holds is reported WITH its identity: the caller has to
	// choose between steering it, answering it and canceling it, and it cannot choose
	// without knowing which run and what state. Waiting counts — a Run parked on a
	// person is still the Session's Run.
	active, err := c.activeRunConflict(ctx, sess.ID())
	if err != nil {
		runAdmission.Release()
		return sessionadmission.RunAdmission{}, err
	}
	if active != nil {
		runAdmission.Release()
		return sessionadmission.RunAdmission{}, active
	}
	return runAdmission, nil
}

// activeRunConflict reports the Session's non-terminal Run as a conflict, or nil when
// it has none. One author, because the same conflict is reachable twice: this process
// can see the Run before admission, and the durable unique index can reject the
// INSERT after another process created one.
func (c *Coordinator) activeRunConflict(ctx context.Context, sessionID string) (error, error) {
	run, found, err := c.activeRuns.ActiveRun(ctx, sessionID)
	if err != nil || !found {
		return nil, err
	}
	return &ActiveRunConflictError{RunID: run.ID(), Status: run.State().Status()}, nil
}

// executionCWD resolves where a Session's tools operate: the sandbox copy
// for an isolated session (created on first use), else the project directory.
// It fails closed when isolation is requested but unavailable — an isolated run
// must never fall back to the real tree.
func (c *Coordinator) executionCWD(ctx context.Context, sess session.Session) (cwd string, isolated bool, err error) {
	if !sess.Isolated() {
		return sess.CWD(), false, nil
	}
	if c.isolation == nil {
		return "", false, fmt.Errorf("%w: isolation is not configured", ErrIsolationUnavailable)
	}
	copyDir, err := c.isolation.Workspace(ctx, sess.ID(), sess.CWD())
	if err != nil {
		return "", false, fmt.Errorf("%w: %w", ErrIsolationUnavailable, err)
	}
	return copyDir, true, nil
}
