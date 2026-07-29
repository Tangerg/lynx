package runs

import (
	"context"
	"errors"
	"iter"
	"runtime"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/admission"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

type fakeRunSessions struct {
	sess session.Session
	// active is the Session's non-terminal Run, when it has one. The zero value is a
	// Session free to start.
	active        *transcript.Run
	createdTitle  string
	pending       map[string]interrupts.Pending
	canceledRunID string
	cancelReason  string
	canceledAt    time.Time
	lostRunID     string
	lostAt        time.Time
	operations    *[]string
}

func (f *fakeRunSessions) Get(context.Context, string) (session.Session, error) {
	return f.sess, nil
}

func (f *fakeRunSessions) ActiveRun(context.Context, string) (transcript.Run, bool, error) {
	if f.active == nil {
		return transcript.Run{}, false, nil
	}
	return *f.active, true, nil
}

func (f *fakeRunSessions) Create(_ context.Context, title, cwd string) (session.Session, error) {
	f.createdTitle = title
	f.sess = session.Session{ID: "ses_created", Cwd: cwd}
	return f.sess, nil
}

func (f *fakeRunSessions) PrepareScheduled(_ context.Context, id, title, cwd string) (session.Session, error) {
	f.createdTitle = title
	f.sess = session.Session{ID: id, Cwd: cwd}
	return f.sess, nil
}

func (f *fakeRunSessions) ListOpenInterrupts(_ context.Context, sessionID string) ([]interrupts.Pending, error) {
	var out []interrupts.Pending
	for _, pending := range f.pending {
		if pending.SessionID == sessionID {
			out = append(out, pending)
		}
	}
	return out, nil
}

func (f *fakeRunSessions) GetOpenInterrupt(_ context.Context, runID string) (interrupts.Pending, bool, error) {
	pending, ok := f.pending[runID]
	return pending, ok, nil
}

func (f *fakeRunSessions) ApplyRunCancel(_ context.Context, sessionID, runID, reason string, finishedAt time.Time) (transcript.Run, error) {
	if f.operations != nil {
		*f.operations = append(*f.operations, "durable.cancel")
	}
	f.canceledRunID = runID
	f.cancelReason = reason
	f.canceledAt = finishedAt
	delete(f.pending, runID)
	outcome := execution.OutcomeCanceled
	return transcript.Run{
		ID: runID, SessionID: sessionID, State: execution.Canceled,
		Outcome: &outcome, Detail: reason, FinishedAt: finishedAt,
	}, nil
}

func (f *fakeRunSessions) ApplyRunLost(_ context.Context, _ string, runID string, finishedAt time.Time) error {
	if f.operations != nil {
		*f.operations = append(*f.operations, "durable.lost")
	}
	f.lostRunID = runID
	f.lostAt = finishedAt
	delete(f.pending, runID)
	return nil
}

type fakeTurnControl struct {
	validated     StartTurn
	started       StartTurn
	startTurn     execution.TurnRef
	prepared      execution.TurnRef
	prepareErr    error
	rehydrated    execution.TurnRef
	rehydrateReq  RehydrateTurn
	rehydrateErr  error
	resumeCheck   func()
	activateCheck func()
	activated     bool
	resumed       bool
	canceled      []execution.TurnRef
	steered       []execution.TurnRef
	steerMessage  string
	operations    *[]string
	cancelErr     error
}

func (f *fakeTurnControl) ValidateStart(req StartTurn) error {
	f.validated = req
	return nil
}

func (f *fakeTurnControl) PrepareStart(_ context.Context, req StartTurn) (execution.TurnRef, error) {
	f.started = req
	return f.startTurn, nil
}

func (f *fakeTurnControl) Activate(context.Context, execution.TurnRef) error {
	if f.activateCheck != nil {
		f.activateCheck()
	}
	f.activated = true
	return nil
}

func (f *fakeTurnControl) Prepare(context.Context, execution.TurnRef) (execution.TurnRef, error) {
	return f.prepared, f.prepareErr
}

func (f *fakeTurnControl) Resume(context.Context, execution.TurnRef, interrupts.Resolution, []execution.InterruptKind) error {
	if f.resumeCheck != nil {
		f.resumeCheck()
	}
	f.resumed = true
	return nil
}

func (f *fakeTurnControl) Rehydrate(_ context.Context, request RehydrateTurn) (execution.TurnRef, error) {
	f.rehydrateReq = request
	return f.rehydrated, f.rehydrateErr
}

func (f *fakeTurnControl) CancelTurn(_ context.Context, ref execution.TurnRef) error {
	if f.operations != nil {
		*f.operations = append(*f.operations, "turn.cancel")
	}
	f.canceled = append(f.canceled, ref)
	return f.cancelErr
}

func (f *fakeTurnControl) Steer(_ context.Context, ref execution.TurnRef, message string) error {
	f.steered = append(f.steered, ref)
	f.steerMessage = message
	return nil
}

func newUseCaseCoordinator(exec SegmentExecutor, turns TurnControl, sessions SessionLifecycle, effects Effects) *Coordinator {
	deps := Dependencies{
		Segments: exec,
		Turns:    turns,
		Sessions: sessions,
		Effects:  effects,
		Runs: &fakeRunProjection{runs: map[string]transcript.Run{
			"run_1":   {ID: "run_1", SessionID: "ses_1", State: execution.Running, ActiveSegmentID: "seg_1"},
			"run_new": {ID: "run_new", SessionID: "ses_1", State: execution.Running, ActiveSegmentID: "seg_new"},
		}},
		Now:          func() time.Time { return time.Date(2026, 7, 13, 1, 2, 3, 0, time.UTC) },
		NewRunID:     func() string { return "run_new" },
		NewSegmentID: func() string { return "seg_new" },
	}
	deps.Admissions = new(admission.Gate)
	return NewCoordinator(deps)
}

func mustUseCaseSelection(provider, model string) modelref.Selection {
	selection, err := modelref.New(provider, model)
	if err != nil {
		panic(err)
	}
	return selection
}

func TestStartOwnsCompleteAdmissionSequence(t *testing.T) {
	exec := &fakeExecutor{}
	effects := &fakeEffects{}
	sessions := &fakeRunSessions{sess: session.Session{ID: "ses_1", Cwd: "/work"}}
	turns := &fakeTurnControl{startTurn: execution.TurnRef{SessionID: "ses_1", TurnID: "turn_1"}}
	activatedAfterOpening := false
	turns.activateCheck = func() { activatedAfterOpening = effects.opening().Admit != nil }
	c := newUseCaseCoordinator(exec, turns, sessions, effects)

	result, err := c.Start(context.Background(), StartCommand{
		SessionID:      "ses_1",
		ModelSelection: mustUseCaseSelection("provider", "model"),
		Input:          []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "hello"}},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	for range result.Events {
	}
	if result.RunID != "run_new" || result.SegmentID != "seg_new" || result.SessionID != "ses_1" {
		t.Fatalf("result = %+v", result)
	}
	if turns.started.SessionID != "ses_1" || turns.started.Cwd != "/work" {
		t.Fatalf("started turn = %+v", turns.started)
	}
	if !turns.activated || !activatedAfterOpening {
		t.Fatalf("activated=%v activatedAfterOpening=%v", turns.activated, activatedAfterOpening)
	}
	if opening := effects.opening(); opening.Admit == nil || opening.Admit.RunID != "run_new" {
		t.Fatalf("opening = %+v, want fresh run admission", opening)
	} else if opening.SessionModel == nil || opening.SessionModel.SessionID != "ses_1" || opening.SessionModel.Model != "model" {
		t.Fatalf("opening session model = %+v, want ses_1/model", opening.SessionModel)
	}
}

func TestStartDoesNotActivateRejectedAdmission(t *testing.T) {
	exec := &fakeExecutor{}
	openingErr := errors.New("opening commit failed")
	effects := &fakeEffects{openingErr: openingErr}
	sessions := &fakeRunSessions{sess: session.Session{ID: "ses_1", Cwd: "/work"}}
	turns := &fakeTurnControl{startTurn: execution.TurnRef{SessionID: "ses_1", TurnID: "turn_1"}}
	c := newUseCaseCoordinator(exec, turns, sessions, effects)

	_, err := c.Start(t.Context(), StartCommand{SessionID: "ses_1", Input: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "hello"}}})
	if !errors.Is(err, openingErr) {
		t.Fatalf("Start error = %v, want opening failure", err)
	}
	if turns.activated {
		t.Fatal("rejected admission activated the prepared turn")
	}
	if exec.cancels() != 1 {
		t.Fatalf("prepared turn cancels = %d, want 1", exec.cancels())
	}
}

func TestStartRejectsPartialScheduledIdentityBeforeSideEffects(t *testing.T) {
	for _, command := range []StartCommand{
		{RunID: "run_1"},
		{NewSessionID: "ses_1"},
		{ScheduleFiring: "fire_1"},
		{RunID: "run_1", NewSessionID: "ses_1", ScheduleFiring: "fire_1", SessionID: "ses_existing"},
	} {
		t.Run("partial", func(t *testing.T) {
			exec := &fakeExecutor{}
			turns := &fakeTurnControl{}
			effects := &fakeEffects{}
			sessions := &fakeRunSessions{sess: session.Session{ID: "ses_existing", Cwd: "/work"}}
			command.Input = []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "hello"}}
			_, err := newUseCaseCoordinator(exec, turns, sessions, effects).Start(t.Context(), command)
			if !errors.Is(err, ErrInvalidScheduledStart) {
				t.Fatalf("Start error = %v, want ErrInvalidScheduledStart", err)
			}
			if turns.started.SessionID != "" || len(effects.openings) != 0 {
				t.Fatalf("partial scheduled identity reached side effects: turn=%+v openings=%d", turns.started, len(effects.openings))
			}
		})
	}
}

func TestFastStartReleaseCannotCrossTerminalMaintenance(t *testing.T) {
	finishStarted := make(chan struct{}, 1)
	releaseFinish := make(chan struct{})
	sessions := &fakeRunSessions{
		sess: session.Session{ID: "ses_1", Cwd: "/work"},
	}
	effects := &fakeEffects{finishStarted: finishStarted, finishRelease: releaseFinish}
	c := newUseCaseCoordinator(
		&fakeExecutor{events: []EngineEvent{TurnEnd{Reason: execution.OutcomeCompleted}}},
		&fakeTurnControl{startTurn: execution.TurnRef{SessionID: "ses_1", TurnID: "turn_1"}},
		sessions,
		effects,
	)

	type startOutcome struct {
		result StartResult
		err    error
	}
	started := make(chan startOutcome, 1)
	go func() {
		result, err := c.Start(t.Context(), StartCommand{SessionID: "ses_1", Input: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "hello"}}})
		started <- startOutcome{result: result, err: err}
	}()

	select {
	case <-finishStarted:
	case <-time.After(time.Second):
		t.Fatal("fast run did not reach terminal maintenance")
	}
	outcome := <-started
	if outcome.err != nil {
		t.Fatalf("Start: %v", outcome.err)
	}
	if !hasActiveSession(c, "ses_1") {
		t.Fatal("Start release erased the in-flight terminal-maintenance claim")
	}
	if release, ok := c.admission.AcquireSession("ses_1"); ok {
		release()
		t.Fatal("new admission crossed terminal maintenance after Start returned")
	}

	close(releaseFinish)
	for range outcome.result.Events {
	}
	requireCoordinatorShutdown(t, c)
	if hasActiveSession(c, "ses_1") {
		t.Fatal("terminal maintenance did not release its claim")
	}
}

func TestStartRejectsForeignTurnIdentityAndCleansItUp(t *testing.T) {
	exec := &fakeExecutor{}
	effects := &fakeEffects{}
	sessions := &fakeRunSessions{sess: session.Session{ID: "ses_1", Cwd: "/work"}}
	turns := &fakeTurnControl{startTurn: execution.TurnRef{SessionID: "ses_foreign", TurnID: "turn_1"}}
	c := newUseCaseCoordinator(exec, turns, sessions, effects)

	_, err := c.Start(context.Background(), StartCommand{SessionID: "ses_1", Input: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "hello"}}})
	if !errors.Is(err, execution.ErrInvalidTurnRef) {
		t.Fatalf("Start error = %v, want ErrInvalidTurnRef", err)
	}
	if len(turns.canceled) != 1 || turns.canceled[0] != turns.startTurn {
		t.Fatalf("canceled turns = %+v, want invalid started turn", turns.canceled)
	}
	if _, ok := c.registry.Get("run_new"); len(effects.openings) != 0 || ok {
		t.Fatal("invalid turn identity reached run admission")
	}
}

func TestResumeCommitsOpeningBeforeActivation(t *testing.T) {
	createdAt := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	effects := &fakeEffects{}
	sessions := &fakeRunSessions{
		sess: session.Session{ID: "ses_1", Cwd: "/work"},
		pending: map[string]interrupts.Pending{"run_1": {
			RootRunID: "run_1", SessionID: "ses_1", TurnID: "turn_1", RunCreatedAt: createdAt,
			Interrupts: approvalInterrupt("item_1"),
		}},
	}
	turns := &fakeTurnControl{prepared: execution.TurnRef{SessionID: "ses_1", TurnID: "turn_1"}}
	activatedAfterOpening := false
	turns.resumeCheck = func() { activatedAfterOpening = effects.opening().Resume != nil }
	c := newUseCaseCoordinator(&fakeExecutor{}, turns, sessions, effects)

	result, err := c.Resume(context.Background(), ResumeCommand{
		RunID: "run_1",
		Responses: []ResumeResponse{{
			ItemID: "item_1", Kind: ApprovalResponseKind,
			Approval: &ApprovalResponse{Approved: true},
		}},
	})
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	for range result.Events {
	}
	if !turns.resumed || !activatedAfterOpening {
		t.Fatalf("resumed=%v activatedAfterOpening=%v", turns.resumed, activatedAfterOpening)
	}
	if opening := effects.opening(); opening.Resume == nil || opening.Resume.RunID != "run_1" {
		t.Fatalf("opening = %+v, want resume run_1", opening)
	}
}

// TestResumeWithInputCommitsTheUserTurnWithTheContinuation is the atomic half of
// "approve, and also do this differently". Before resume could carry input, that was
// two calls — resume then steer — with a window between them where the model could
// finish the tool round before the instruction ever arrived.
//
// The user Item rides the SAME opening write-set as the continuation, so either both
// landed or neither did, and the response names the item only when there is one: that
// iff is a cross-shape rule no schema keyword can state, so it is held here.
func TestResumeWithInputCommitsTheUserTurnWithTheContinuation(t *testing.T) {
	createdAt := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	newResumeCase := func() (*fakeEffects, *Coordinator) {
		effects := &fakeEffects{}
		sessions := &fakeRunSessions{
			sess: session.Session{ID: "ses_1", Cwd: "/work"},
			pending: map[string]interrupts.Pending{"run_1": {
				RootRunID: "run_1", SessionID: "ses_1", TurnID: "turn_1", RunCreatedAt: createdAt,
				Interrupts: approvalInterrupt("item_1"),
			}},
		}
		turns := &fakeTurnControl{prepared: execution.TurnRef{SessionID: "ses_1", TurnID: "turn_1"}}
		return effects, newUseCaseCoordinator(&fakeExecutor{}, turns, sessions, effects)
	}
	approve := []ResumeResponse{{
		ItemID: "item_1", Kind: ApprovalResponseKind,
		Approval: &ApprovalResponse{Approved: true},
	}}

	effects, c := newResumeCase()
	withInput, err := c.Resume(context.Background(), ResumeCommand{
		RunID: "run_1", Responses: approve,
		Input: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "also skip the tests"}},
	})
	if err != nil {
		t.Fatalf("Resume with input: %v", err)
	}
	for range withInput.Events {
	}
	if withInput.UserItemID == "" {
		t.Fatal("a resume that carried input named no user item")
	}
	opening := effects.opening()
	if opening.Resume == nil {
		t.Fatalf("opening = %+v, want the continuation", opening)
	}
	committed := false
	for _, event := range opening.Events {
		for _, item := range event.Items {
			if item.ID == withInput.UserItemID && item.Kind == transcript.UserMessage {
				committed = true
			}
		}
	}
	if !committed {
		t.Fatalf("the user item is not in the continuation's write-set: %+v", opening.Events)
	}

	_, c = newResumeCase()
	without, err := c.Resume(context.Background(), ResumeCommand{RunID: "run_1", Responses: approve})
	if err != nil {
		t.Fatalf("Resume without input: %v", err)
	}
	for range without.Events {
	}
	if without.UserItemID != "" {
		t.Fatalf("userItemId = %q on a resume that opened no user turn", without.UserItemID)
	}
}

func TestResumeRecoversLostProcessSnapshotBeforeReturning(t *testing.T) {
	var operations []string
	sessions := &fakeRunSessions{
		sess: session.Session{ID: "ses_1", Cwd: "/work"},
		pending: map[string]interrupts.Pending{"run_1": {
			RootRunID: "run_1", SessionID: "ses_1", TurnID: "turn_1", ProcessID: "proc_1",
			Interrupts: approvalInterrupt("item_1"),
		}},
		operations: &operations,
	}
	turns := &fakeTurnControl{
		prepareErr:   ErrTurnNotLive,
		rehydrateErr: ErrTurnStateLost,
	}
	c := newUseCaseCoordinator(&fakeExecutor{}, turns, sessions, &fakeEffects{})

	_, err := c.Resume(t.Context(), ResumeCommand{
		RunID: "run_1",
		Responses: []ResumeResponse{{
			ItemID: "item_1", Kind: ApprovalResponseKind,
			Approval: &ApprovalResponse{Approved: true},
		}},
	})
	if !errors.Is(err, ErrRunNotFound) || !errors.Is(err, ErrTurnStateLost) {
		t.Fatalf("Resume error = %v, want run not found wrapping turn state lost", err)
	}
	if sessions.lostRunID != "run_1" || sessions.lostAt.IsZero() {
		t.Fatalf("lost recovery = %q/%v, want run_1 and terminal time", sessions.lostRunID, sessions.lostAt)
	}
	if len(operations) != 1 || operations[0] != "durable.lost" {
		t.Fatalf("operations = %v, want one durable lost commit", operations)
	}
	if turns.rehydrateReq.Cwd != "/work" {
		t.Fatalf("rehydrate cwd = %q, want /work", turns.rehydrateReq.Cwd)
	}
	if hasActiveSession(c, "ses_1") {
		t.Fatal("failed resume leaked its run admission")
	}

	_, err = c.Resume(t.Context(), ResumeCommand{RunID: "run_1"})
	if !errors.Is(err, ErrInterruptNotOpen) {
		t.Fatalf("second Resume error = %v, want ErrInterruptNotOpen", err)
	}
	if len(operations) != 1 {
		t.Fatalf("second Resume repeated recovery: %v", operations)
	}
}

func TestResumeRefusesIsolatedRunAfterSandboxProcessEnded(t *testing.T) {
	var operations []string
	sessions := &fakeRunSessions{
		sess: session.Session{ID: "ses_1", Cwd: "/work", Isolated: true},
		pending: map[string]interrupts.Pending{"run_1": {
			RootRunID: "run_1", SessionID: "ses_1", TurnID: "turn_1", ProcessID: "proc_1",
			Interrupts: approvalInterrupt("item_1"),
		}},
		operations: &operations,
	}
	// The process that owned the sandbox copy is gone (Prepare reports the turn as
	// not live), so a rehydrate would run against the real project tree.
	turns := &fakeTurnControl{prepareErr: ErrTurnNotLive}
	c := newUseCaseCoordinator(&fakeExecutor{}, turns, sessions, &fakeEffects{})

	_, err := c.Resume(t.Context(), ResumeCommand{
		RunID: "run_1",
		Responses: []ResumeResponse{{
			ItemID: "item_1", Kind: ApprovalResponseKind,
			Approval: &ApprovalResponse{Approved: true},
		}},
	})
	if !errors.Is(err, ErrRunNotFound) || !errors.Is(err, ErrTurnStateLost) {
		t.Fatalf("Resume error = %v, want run not found wrapping turn state lost", err)
	}
	if turns.rehydrateReq != (RehydrateTurn{}) {
		t.Fatalf("isolated run was rehydrated against %+v, want no rehydrate", turns.rehydrateReq)
	}
	if sessions.lostRunID != "run_1" || len(operations) != 1 || operations[0] != "durable.lost" {
		t.Fatalf("lost recovery = %q ops=%v, want run_1 marked lost", sessions.lostRunID, operations)
	}
	if hasActiveSession(c, "ses_1") {
		t.Fatal("failed isolated resume leaked its run admission")
	}
}

func approvalInterrupt(itemID string) []transcript.Interrupt {
	return []transcript.Interrupt{{
		ItemID: itemID,
		Kind:   execution.ApprovalInterrupt,
		Approval: &transcript.Approval{
			Tool: transcript.ToolInvocation{Name: "shell"},
		},
	}}
}

func TestCancelParkedRunUsesApplicationAdmission(t *testing.T) {
	var operations []string
	sessions := &fakeRunSessions{pending: map[string]interrupts.Pending{"run_1": {
		RootRunID: "run_1", SessionID: "ses_1", TurnID: "turn_1",
	}}, operations: &operations}
	turns := &fakeTurnControl{operations: &operations}
	c := NewCoordinator(Dependencies{
		Turns: turns, Sessions: sessions,
		Runs: &fakeRunProjection{runs: map[string]transcript.Run{
			"run_1": {ID: "run_1", SessionID: "ses_1", State: execution.Interrupted},
		}},
		Admissions: new(admission.Gate),
	})

	result, err := c.Cancel(t.Context(), CancelCommand{RunID: "run_1", Reason: "user stopped"})
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if result.Run.ID != "run_1" || result.Run.State != execution.Canceled {
		t.Fatalf("Cancel result = %+v, want canceled run_1", result)
	}
	if sessions.canceledRunID != "run_1" || len(turns.canceled) != 1 {
		t.Fatalf("durable cancel=%q turn cancels=%v", sessions.canceledRunID, turns.canceled)
	}
	if sessions.cancelReason != "user stopped" || sessions.canceledAt.IsZero() {
		t.Fatalf("cancel reason/time = %q/%v, want user reason and terminal time", sessions.cancelReason, sessions.canceledAt)
	}
	if len(operations) != 2 || operations[0] != "durable.cancel" || operations[1] != "turn.cancel" {
		t.Fatalf("cancel operations = %v, want durable commit before process cleanup", operations)
	}
	if hasActiveSession(c, "ses_1") {
		t.Fatal("parked cancel leaked the session admission claim")
	}
}

func TestCancelFinishedRunReportsFinishedInsteadOfNotFound(t *testing.T) {
	finished := runRecord(execution.Completed, "", "")
	c := NewCoordinator(Dependencies{
		Turns:      &fakeTurnControl{},
		Sessions:   &fakeRunSessions{},
		Runs:       &fakeRunProjection{runs: map[string]transcript.Run{"run_1": finished}},
		Admissions: new(admission.Gate),
	})

	_, err := c.Cancel(t.Context(), CancelCommand{RunID: "run_1", Reason: "too late"})
	if !errors.Is(err, ErrRunFinished) {
		t.Fatalf("Cancel error = %v, want ErrRunFinished", err)
	}
}

func TestCancelUnknownRunReportsNotFound(t *testing.T) {
	c := NewCoordinator(Dependencies{
		Turns:      &fakeTurnControl{},
		Sessions:   &fakeRunSessions{},
		Runs:       &fakeRunProjection{},
		Admissions: new(admission.Gate),
	})

	_, err := c.Cancel(t.Context(), CancelCommand{RunID: "run_missing"})
	if !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("Cancel error = %v, want ErrRunNotFound", err)
	}
}

func TestCancelChildRunRequiresExplicitAuthority(t *testing.T) {
	child := runRecord(execution.Running, "seg_child", "item_parent")
	c := NewCoordinator(Dependencies{
		Turns:      &fakeTurnControl{},
		Sessions:   &fakeRunSessions{},
		Runs:       &fakeRunProjection{runs: map[string]transcript.Run{"run_1": child}},
		Admissions: new(admission.Gate),
	})

	_, err := c.Cancel(t.Context(), CancelCommand{RunID: "run_1"})
	if !errors.Is(err, ErrChildRunNotAllowed) {
		t.Fatalf("Cancel error = %v, want ErrChildRunNotAllowed", err)
	}
}

func TestCancelParkedRunReportsTurnCleanupFailureAfterDurableCommit(t *testing.T) {
	cleanupErr := errors.New("turn cleanup failed")
	sessions := &fakeRunSessions{pending: map[string]interrupts.Pending{"run_1": {
		RootRunID: "run_1", SessionID: "ses_1", TurnID: "turn_1",
	}}}
	turns := &fakeTurnControl{cancelErr: cleanupErr}
	c := NewCoordinator(Dependencies{
		Turns: turns, Sessions: sessions,
		Runs: &fakeRunProjection{runs: map[string]transcript.Run{
			"run_1": {ID: "run_1", SessionID: "ses_1", State: execution.Interrupted},
		}},
		Admissions: new(admission.Gate),
	})

	_, err := c.Cancel(t.Context(), CancelCommand{RunID: "run_1", Reason: "stop"})
	if !errors.Is(err, cleanupErr) {
		t.Fatalf("Cancel error = %v, want cleanup failure", err)
	}
	if sessions.canceledRunID != "run_1" {
		t.Fatal("turn cleanup failure prevented the durable cancel commit")
	}
}

func TestCancelLiveRunReportsTurnCleanupFailureAndStillTerminalizes(t *testing.T) {
	cleanupErr := errors.New("turn cleanup failed")
	executor := &fakeExecutor{block: true, cancelErr: cleanupErr}
	effects := &fakeEffects{}
	turns := &fakeTurnControl{}
	c := NewCoordinator(Dependencies{
		Segments: executor, Turns: turns, Sessions: &fakeRunSessions{}, Effects: effects,
		Runs: &fakeRunProjection{runs: map[string]transcript.Run{
			"run_1": {ID: "run_1", SessionID: "ses_1", State: execution.Running, ActiveSegmentID: "seg_1"},
		}},
		Admissions: new(admission.Gate),
	})
	stream, err := c.openSegment(t.Context(), testSegment())
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	next, stop := iter.Pull(stream)
	defer stop()
	next() // consume the opening event so the pump is live

	_, err = c.Cancel(t.Context(), CancelCommand{RunID: "run_1", Reason: "stop"})
	if !errors.Is(err, cleanupErr) {
		t.Fatalf("Cancel error = %v, want cleanup failure", err)
	}
	for _, ok := next(); ok; _, ok = next() { // drain the terminal events
	}
	if !effects.terminalized("ses_1", "run_1") {
		t.Fatal("turn cleanup failure prevented live run terminalization")
	}
}

func TestCancelLiveRunJoinsTerminalMaintenance(t *testing.T) {
	finishStarted := make(chan struct{}, 1)
	releaseFinish := make(chan struct{})
	executor := &fakeExecutor{block: true}
	effects := &fakeEffects{finishStarted: finishStarted, finishRelease: releaseFinish}
	turns := &fakeTurnControl{startTurn: execution.TurnRef{SessionID: "ses_1", TurnID: "turn_1"}}
	sessions := &fakeRunSessions{sess: session.Session{ID: "ses_1", Cwd: "/work"}}
	c := newUseCaseCoordinator(executor, turns, sessions, effects)
	result, err := c.Start(t.Context(), StartCommand{
		SessionID: "ses_1",
		Input:     []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "hello"}},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	type cancelOutcome struct {
		result CancelResult
		err    error
	}
	cancelDone := make(chan cancelOutcome, 1)
	go func() {
		canceled, cancelErr := c.Cancel(t.Context(), CancelCommand{RunID: result.RunID, Reason: "stop"})
		cancelDone <- cancelOutcome{result: canceled, err: cancelErr}
	}()
	select {
	case <-finishStarted:
	case <-time.After(time.Second):
		t.Fatal("canceled run did not reach terminal maintenance")
	}
	select {
	case outcome := <-cancelDone:
		t.Fatalf("Cancel returned before terminal maintenance: result=%+v err=%v", outcome.result, outcome.err)
	default:
	}

	close(releaseFinish)
	outcome := <-cancelDone
	if outcome.err != nil {
		t.Fatalf("Cancel: %v", outcome.err)
	}
	if outcome.result.Run.ID != result.RunID ||
		outcome.result.Run.State != execution.Canceled ||
		outcome.result.Run.Outcome == nil ||
		*outcome.result.Run.Outcome != execution.OutcomeCanceled ||
		outcome.result.Run.Detail != "stop" {
		t.Fatalf("Cancel result = %+v, want exact canceled terminal snapshot", outcome.result)
	}
	if hasActiveSession(c, "ses_1") {
		t.Fatal("Cancel returned before releasing session admission")
	}
	for range result.Events {
	}
}

func TestCancelLosesToACommittedNaturalTerminal(t *testing.T) {
	terminalStarted := make(chan struct{}, 1)
	releaseTerminal := make(chan struct{})
	executor := &fakeExecutor{events: []EngineEvent{TurnEnd{
		Reason: execution.OutcomeCompleted,
	}}}
	effects := &fakeEffects{terminalStarted: terminalStarted, terminalRelease: releaseTerminal}
	c := NewCoordinator(Dependencies{
		Segments: executor, Turns: &fakeTurnControl{}, Sessions: &fakeRunSessions{}, Effects: effects,
		Runs: &fakeRunProjection{runs: map[string]transcript.Run{
			"run_1": {ID: "run_1", SessionID: "ses_1", State: execution.Running, ActiveSegmentID: "seg_1"},
		}},
		Admissions: new(admission.Gate),
	})
	stream, err := c.openSegment(t.Context(), testSegment())
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	streamDone := make(chan struct{})
	go func() {
		collectEvents(stream)
		close(streamDone)
	}()
	select {
	case <-terminalStarted:
	case <-time.After(time.Second):
		t.Fatal("natural terminal commit did not start")
	}

	cancelDone := make(chan error, 1)
	go func() {
		_, cancelErr := c.Cancel(t.Context(), CancelCommand{RunID: "run_1", Reason: "too late"})
		cancelDone <- cancelErr
	}()
	entry, live := c.registry.Get("run_1")
	if !live {
		t.Fatal("terminal commit lost its live cancellation join")
	}
	deadline := time.After(time.Second)
	for entry.handle.CancelReason() != "too late" {
		select {
		case <-deadline:
			t.Fatal("cancel did not join the in-flight terminal commit")
		default:
			runtime.Gosched()
		}
	}
	close(releaseTerminal)
	if err := <-cancelDone; !errors.Is(err, ErrRunFinished) {
		t.Fatalf("Cancel error = %v, want ErrRunFinished", err)
	}
	<-streamDone
}

func TestCancelLetsCommittedInterruptOwnDurableFirstTeardown(t *testing.T) {
	suspendStarted := make(chan struct{}, 1)
	suspendCanceled := make(chan struct{}, 1)
	releaseSuspend := make(chan struct{})
	executor := &fakeExecutor{events: []EngineEvent{
		ToolCallStart{
			CallID: "call_1", ToolName: "shell", Arguments: `{"command":"pwd"}`,
			SafetyClass: "write",
		},
		TurnInterrupted{Interrupts: []Interrupt{{
			Kind: execution.ApprovalInterrupt,
			Approval: &ApprovalPrompt{
				CallID: "call_1", ToolName: "shell", Arguments: `{"command":"pwd"}`,
				SafetyClass: "write",
			},
		}}},
	}}
	effects := &fakeEffects{
		suspendStarted: suspendStarted, suspendCanceled: suspendCanceled,
		suspendRelease: releaseSuspend,
	}
	var operations []string
	turns := &fakeTurnControl{operations: &operations}
	sessions := &fakeRunSessions{operations: &operations}
	c := NewCoordinator(Dependencies{
		Segments: executor, Turns: turns, Sessions: sessions, Effects: effects,
		Runs: &fakeRunProjection{runs: map[string]transcript.Run{
			"run_1": {ID: "run_1", SessionID: "ses_1", State: execution.Running, ActiveSegmentID: "seg_1"},
		}},
		Admissions: new(admission.Gate),
	})
	stream, err := c.openSegment(t.Context(), testSegment())
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	streamDone := make(chan struct{})
	go func() {
		collectEvents(stream)
		close(streamDone)
	}()
	select {
	case <-suspendStarted:
	case <-time.After(time.Second):
		t.Fatal("interrupt commit did not start")
	}

	cancelDone := make(chan error, 1)
	go func() {
		_, cancelErr := c.Cancel(t.Context(), CancelCommand{RunID: "run_1", Reason: "stop"})
		cancelDone <- cancelErr
	}()
	select {
	case <-suspendCanceled:
	case <-time.After(time.Second):
		t.Fatal("cancel did not reach the in-flight interrupt commit")
	}
	close(releaseSuspend)
	if err := <-cancelDone; err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	<-streamDone

	if executor.cancels() != 0 {
		t.Fatalf("pump executor cancellations = %d, want parked owner to remain intact until durable cancel", executor.cancels())
	}
	if len(operations) != 2 || operations[0] != "durable.cancel" || operations[1] != "turn.cancel" {
		t.Fatalf("cancel operations = %v, want durable cancel before parked turn cleanup", operations)
	}
}

func TestCancelTreatsAlreadyGoneTurnAsIdempotentSuccess(t *testing.T) {
	sessions := &fakeRunSessions{pending: map[string]interrupts.Pending{"run_1": {
		RootRunID: "run_1", SessionID: "ses_1", TurnID: "turn_1",
	}}}
	turns := &fakeTurnControl{cancelErr: ErrTurnNotLive}
	c := NewCoordinator(Dependencies{
		Turns: turns, Sessions: sessions,
		Runs: &fakeRunProjection{runs: map[string]transcript.Run{
			"run_1": {ID: "run_1", SessionID: "ses_1", State: execution.Interrupted},
		}},
		Admissions: new(admission.Gate),
	})

	if _, err := c.Cancel(t.Context(), CancelCommand{RunID: "run_1"}); err != nil {
		t.Fatalf("Cancel error = %v, want idempotent success", err)
	}
}

func TestSteerHidesExecutorHandle(t *testing.T) {
	turns := &fakeTurnControl{}
	c, _ := liveCoordinator(t, runRecord(execution.Running, testSegmentID, ""))
	c.turns = turns

	if err := c.Steer(context.Background(), SteerCommand{
		RunID: testRunID, ExpectedSegmentID: testSegmentID, Message: "wait",
	}); err != nil {
		t.Fatalf("Steer: %v", err)
	}
	if len(turns.steered) != 1 || turns.steered[0] != (execution.TurnRef{SessionID: "ses_1", TurnID: "turn_1"}) {
		t.Fatalf("steered refs = %+v", turns.steered)
	}
	if turns.steerMessage != "wait" {
		t.Fatalf("steer message = %q", turns.steerMessage)
	}
}

func TestStartRejectsInvalidInputBeforeSessionCreation(t *testing.T) {
	sessions := &fakeRunSessions{}
	c := newUseCaseCoordinator(&fakeExecutor{}, &fakeTurnControl{}, sessions, &fakeEffects{})

	_, err := c.Start(context.Background(), StartCommand{})
	if !errors.Is(err, ErrInputRequired) {
		t.Fatalf("err = %v, want ErrInputRequired", err)
	}
	if sessions.sess.ID != "" {
		t.Fatalf("invalid input created session %+v", sessions.sess)
	}
}

// TestStartRefusesASessionThatAlreadyHasARunAndNamesIt is the admission conflict the
// contract makes typed: nothing is created, nothing is canceled, and the refusal
// carries the run so the caller can choose between steering it, answering it and
// canceling it.
//
// The alternative — an implicit cancel — throws away work to serve a request that may
// have been meant as a steer, and the runtime cannot tell which the person wanted.
func TestStartRefusesASessionThatAlreadyHasARunAndNamesIt(t *testing.T) {
	for _, tt := range []struct {
		name   string
		state  execution.RunState
		status execution.RunStatus
	}{
		{"a running run", execution.Running, execution.StatusRunning},
		{"a run waiting on a person", execution.Interrupted, execution.StatusWaiting},
	} {
		t.Run(tt.name, func(t *testing.T) {
			effects := &fakeEffects{}
			sessions := &fakeRunSessions{
				sess:   session.Session{ID: "ses_1", Cwd: "/work"},
				active: &transcript.Run{ID: "run_active", SessionID: "ses_1", State: tt.state},
			}
			c := newUseCaseCoordinator(&fakeExecutor{}, &fakeTurnControl{}, sessions, effects)

			_, err := c.Start(context.Background(), StartCommand{
				SessionID:      "ses_1",
				ModelSelection: mustUseCaseSelection("provider", "model"),
				Input:          []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "hi"}},
			})
			conflict, ok := errors.AsType[*ActiveRunConflict](err)
			if !ok {
				t.Fatalf("Start = %v, want an ActiveRunConflict", err)
			}
			if conflict.RunID != "run_active" || conflict.Status != tt.status {
				t.Fatalf("conflict = %+v, want run_active as %s", conflict, tt.status)
			}
			if opening := effects.opening(); opening.Admit != nil {
				t.Fatal("a refused start committed an opening — nothing may be created")
			}
			if sessions.canceledRunID != "" {
				t.Fatalf("a refused start canceled %q — the runtime never chooses for the user", sessions.canceledRunID)
			}
		})
	}
}
