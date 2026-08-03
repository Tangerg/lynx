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
	validated      StartTurn
	started        StartTurn
	startTurn      execution.TurnRef
	prepared       execution.TurnRef
	prepareErr     error
	rehydrated     execution.TurnRef
	rehydrateReq   RehydrateTurn
	rehydrateErr   error
	resumeCheck    func()
	activateCheck  func()
	activated      bool
	resumed        bool
	canceled       []execution.TurnRef
	steered        []execution.TurnRef
	steerInput     []transcript.ContentBlock
	operations     *[]string
	cancelErr      error
	cancelSubtree  func(execution.TurnRef, string) error
	prepareWaiting func(execution.TurnRef, string) (PreparedWaitingSubtreeCancellation, error)
}

type blockingOpeningEffects struct {
	*fakeEffects
	started chan<- struct{}
	release <-chan struct{}
}

func (e *blockingOpeningEffects) CommitOpening(
	ctx context.Context,
	opening OpeningCommit,
) error {
	e.started <- struct{}{}
	select {
	case <-e.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	return e.fakeEffects.CommitOpening(ctx, opening)
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

func (f *fakeTurnControl) Resume(context.Context, execution.TurnRef, []interrupts.SuspensionAnswer, []execution.InterruptKind) error {
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

func (f *fakeTurnControl) CancelSubtree(_ context.Context, ref execution.TurnRef, processID string) error {
	if f.operations != nil {
		*f.operations = append(*f.operations, "turn.cancel_subtree:"+processID)
	}
	f.canceled = append(f.canceled, ref)
	if f.cancelSubtree != nil {
		return f.cancelSubtree(ref, processID)
	}
	return f.cancelErr
}

func (f *fakeTurnControl) PrepareWaitingSubtreeCancellation(
	_ context.Context,
	ref execution.TurnRef,
	processID string,
) (PreparedWaitingSubtreeCancellation, error) {
	if f.prepareWaiting == nil {
		return PreparedWaitingSubtreeCancellation{}, errors.New("fake turn control: waiting subtree cancellation is not configured")
	}
	return f.prepareWaiting(ref, processID)
}

func (f *fakeTurnControl) Steer(_ context.Context, ref execution.TurnRef, input []transcript.ContentBlock) error {
	f.steered = append(f.steered, ref)
	f.steerInput = append([]transcript.ContentBlock(nil), input...)
	return nil
}

func newUseCaseCoordinator(exec SegmentExecutor, turns TurnControl, sessions SessionLifecycle, effects Effects) *Coordinator {
	freshCreatedAt := time.Date(2026, 7, 13, 1, 2, 3, 0, time.UTC)
	projection := &fakeRunProjection{runs: map[string]transcript.Run{
		"run_1": runForSegment(testSegment()),
		"run_new": runForSegment(segmentSpec{
			RunID: "run_new", SegmentID: "seg_new", SessionID: "ses_1",
			TurnID: "turn_1", CreatedAt: freshCreatedAt,
		}),
	}}
	if fake, ok := sessions.(*fakeRunSessions); ok {
		for _, pending := range fake.pending {
			for _, continuation := range pending.Continuations {
				projection.runs[continuation.RunID] = runForContinuation(pending, continuation)
			}
		}
	}
	deps := Dependencies{
		Segments:     exec,
		Turns:        turns,
		Sessions:     sessions,
		Effects:      effects,
		Runs:         projection,
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

func TestWaitSessionStartableResolvesWorkingTreeBoundary(t *testing.T) {
	sessions := &fakeRunSessions{sess: session.Session{ID: "ses_1", Cwd: "/work"}}
	c := newUseCaseCoordinator(&fakeExecutor{}, &fakeTurnControl{}, sessions, &fakeEffects{})
	release, ok := c.admission.AcquireWorkingTreeMutation("/work")
	if !ok {
		t.Fatal("acquire working-tree mutation")
	}

	done := make(chan error, 1)
	go func() { done <- c.WaitSessionStartable(t.Context(), "ses_1") }()
	select {
	case err := <-done:
		t.Fatalf("WaitSessionStartable returned inside working-tree mutation: %v", err)
	default:
	}
	release()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("WaitSessionStartable: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("WaitSessionStartable did not observe the session working-tree release")
	}
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
		SessionID:       "ses_1",
		ModelSelection:  mustUseCaseSelection("provider", "model"),
		Limits:          execution.RunLimits{MaxTotalTokens: 16_384, MaxSteps: 12, MaxBudgetUSD: 3.5},
		ProtocolProfile: execution.RunProtocolProfile{ChildRuns: true},
		Input:           []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "hello"}},
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
	wantLimits := execution.RunLimits{MaxTotalTokens: 16_384, MaxSteps: 12, MaxBudgetUSD: 3.5}
	if turns.started.Limits != wantLimits {
		t.Fatalf("executor limits = %+v, want %+v", turns.started.Limits, wantLimits)
	}
	if !turns.validated.ChildRunAdmissionEnabled || !turns.started.ChildRunAdmissionEnabled {
		t.Fatalf("child admission policy did not reach the executor: validated=%+v started=%+v", turns.validated, turns.started)
	}
	if !turns.activated || !activatedAfterOpening {
		t.Fatalf("activated=%v activatedAfterOpening=%v", turns.activated, activatedAfterOpening)
	}
	if opening := effects.opening(); opening.Admit == nil || opening.Admit.RunID != "run_new" {
		t.Fatalf("opening = %+v, want fresh run admission", opening)
	} else if opening.Admit.Limits != wantLimits {
		t.Fatalf("opening limits = %+v, want %+v", opening.Admit.Limits, wantLimits)
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
		&fakeExecutor{events: []ExecutorPayload{TurnEnd{Reason: execution.OutcomeCompleted}}},
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
		pending: map[string]interrupts.Pending{
			"run_1": testPendingInterrupt("item_1", "proc_1", createdAt),
		},
	}
	turns := &fakeTurnControl{prepared: execution.TurnRef{SessionID: "ses_1", TurnID: "turn_1"}}
	activatedAfterOpening := false
	turns.resumeCheck = func() { activatedAfterOpening = effects.opening().Resume != nil }
	c := newUseCaseCoordinator(&fakeExecutor{}, turns, sessions, effects)

	result, err := c.Resume(context.Background(), ResumeCommand{
		RunID: "run_1",
		CallerCapabilities: execution.RunProtocolProfile{
			InterruptKinds: []execution.InterruptKind{execution.ApprovalInterrupt},
		},
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
	if opening := effects.opening(); opening.Resume == nil || opening.Resume.RootRunID != "run_1" {
		t.Fatalf("opening = %+v, want resume run_1", opening)
	}
}

// TestResumeRejectsContinuationFactDriftBeforeExecutorPreparation proves
// parked_continuation_matches_run_facts at segment opening: Pending cannot
// supply a different accounting snapshot before the executor is prepared.
func TestResumeRejectsContinuationFactDriftBeforeExecutorPreparation(t *testing.T) {
	createdAt := time.Date(2026, 7, 30, 11, 0, 0, 0, time.UTC)
	pending := testPendingInterrupt("item_1", "process_root", createdAt)
	sessions := &fakeRunSessions{
		sess: session.Session{ID: pending.SessionID, Cwd: "/work"},
		pending: map[string]interrupts.Pending{
			pending.RootRunID: pending,
		},
	}
	turns := &fakeTurnControl{prepared: execution.TurnRef{
		SessionID: pending.SessionID,
		TurnID:    pending.TurnID,
	}}
	effects := &fakeEffects{}
	coordinator := newUseCaseCoordinator(&fakeExecutor{}, turns, sessions, effects)
	contradictory := runForPending(pending)
	contradictory.Metrics.Steps++
	coordinator.runs = &fakeRunProjection{runs: map[string]transcript.Run{
		pending.RootRunID: contradictory,
	}}

	_, err := coordinator.Resume(t.Context(), ResumeCommand{
		RunID:              pending.RootRunID,
		CallerCapabilities: pending.ProtocolProfile,
		Responses: []ResumeResponse{{
			ItemID: "item_1", Kind: ApprovalResponseKind,
			Approval: &ApprovalResponse{Approved: true},
		}},
	})
	if err == nil {
		t.Fatal("Resume accepted cumulative metrics that differ from the durable Run")
	}
	if turns.resumed || turns.rehydrateReq.ProcessID != "" || len(effects.openings) != 0 {
		t.Fatalf("contradictory continuation reached executor/effects: turns=%+v openings=%d", turns, len(effects.openings))
	}
	if _, found := sessions.pending[pending.RootRunID]; !found {
		t.Fatal("failed validation consumed the open Pending set")
	}
}

func TestResumeAndRootCancelShareOneApplicationAdmissionBoundary(t *testing.T) {
	createdAt := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	pending := testPendingInterrupt("item_1", "proc_1", createdAt)
	sessions := &fakeRunSessions{
		sess: session.Session{ID: "ses_1", Cwd: "/work"},
		pending: map[string]interrupts.Pending{
			"run_1": pending,
		},
	}
	turns := &fakeTurnControl{
		prepared: execution.TurnRef{SessionID: "ses_1", TurnID: "turn_1"},
	}
	openingStarted := make(chan struct{}, 1)
	releaseOpening := make(chan struct{})
	effects := &blockingOpeningEffects{
		fakeEffects: &fakeEffects{},
		started:     openingStarted,
		release:     releaseOpening,
	}
	c := newUseCaseCoordinator(&fakeExecutor{}, turns, sessions, effects)
	c.runs = &fakeRunProjection{runs: map[string]transcript.Run{
		"run_1": runForPending(pending),
	}}

	type resumeOutcome struct {
		result StartResult
		err    error
	}
	resumeDone := make(chan resumeOutcome, 1)
	go func() {
		result, err := c.Resume(t.Context(), ResumeCommand{
			RunID: "run_1",
			CallerCapabilities: execution.RunProtocolProfile{
				InterruptKinds: []execution.InterruptKind{execution.ApprovalInterrupt},
			},
			Responses: []ResumeResponse{{
				ItemID: "item_1",
				Kind:   ApprovalResponseKind,
				Approval: &ApprovalResponse{
					Approved: true,
				},
			}},
		})
		resumeDone <- resumeOutcome{result: result, err: err}
	}()

	select {
	case <-openingStarted:
	case <-time.After(time.Second):
		t.Fatal("resume did not reach its durable opening boundary")
	}
	if _, err := c.Cancel(
		t.Context(),
		CancelCommand{RunID: "run_1", Reason: "racing cancel"},
	); !errors.Is(err, ErrSessionBusy) {
		t.Fatalf("Cancel while resume owns admission = %v, want ErrSessionBusy", err)
	}
	if sessions.canceledRunID != "" {
		t.Fatalf("losing cancel committed run %q", sessions.canceledRunID)
	}

	close(releaseOpening)
	outcome := <-resumeDone
	if outcome.err != nil {
		t.Fatalf("Resume: %v", outcome.err)
	}
	for range outcome.result.Events {
	}
	if !turns.resumed {
		t.Fatal("winning resume did not activate its continuation")
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
			pending: map[string]interrupts.Pending{
				"run_1": testPendingInterrupt("item_1", "proc_1", createdAt),
			},
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
		CallerCapabilities: execution.RunProtocolProfile{
			InterruptKinds: []execution.InterruptKind{execution.ApprovalInterrupt},
		},
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
	without, err := c.Resume(context.Background(), ResumeCommand{
		RunID: "run_1", Responses: approve,
		CallerCapabilities: execution.RunProtocolProfile{
			InterruptKinds: []execution.InterruptKind{execution.ApprovalInterrupt},
		},
	})
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
		pending: map[string]interrupts.Pending{
			"run_1": testPendingInterrupt("item_1", "proc_1", time.Now().UTC()),
		},
		operations: &operations,
	}
	turns := &fakeTurnControl{
		prepareErr:   ErrTurnNotLive,
		rehydrateErr: ErrTurnStateLost,
	}
	c := newUseCaseCoordinator(&fakeExecutor{}, turns, sessions, &fakeEffects{})

	_, err := c.Resume(t.Context(), ResumeCommand{
		RunID: "run_1",
		CallerCapabilities: execution.RunProtocolProfile{
			InterruptKinds: []execution.InterruptKind{execution.ApprovalInterrupt},
		},
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

func TestResumeRehydrateRestoresChildSourceProjection(t *testing.T) {
	createdAt := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	pending := resumedTreePending(createdAt)
	pending.ProtocolProfile.ChildRuns = true
	pending.GoalLeaseID = "goal-lease-1"
	sessions := &fakeRunSessions{
		sess: session.Session{ID: pending.SessionID, Cwd: "/work"},
		pending: map[string]interrupts.Pending{
			pending.RootRunID: pending,
		},
	}
	turns := &fakeTurnControl{
		prepareErr: ErrTurnNotLive,
		rehydrated: execution.TurnRef{
			SessionID: pending.SessionID,
			TurnID:    pending.TurnID,
		},
	}
	c := newUseCaseCoordinator(&fakeExecutor{}, turns, sessions, &fakeEffects{})
	segmentIDs := []string{"segment_root", "segment_grandchild", "segment_a", "segment_b"}
	c.newSegmentID = func() string {
		next := segmentIDs[0]
		segmentIDs = segmentIDs[1:]
		return next
	}
	result, err := c.Resume(t.Context(), ResumeCommand{
		RunID:              pending.RootRunID,
		CallerCapabilities: pending.ProtocolProfile,
		Responses: []ResumeResponse{
			{
				ItemID:   "item_grandchild",
				Kind:     QuestionResponseKind,
				Question: &QuestionResponse{Answers: [][]string{{"continue grandchild"}}},
			},
			{
				ItemID:   "item_b",
				Kind:     QuestionResponseKind,
				Question: &QuestionResponse{Answers: [][]string{{"continue sibling"}}},
			},
		},
	})
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	for range result.Events {
	}
	if !turns.rehydrateReq.ChildRunAdmissionEnabled {
		t.Fatalf("rehydrate request = %+v, want child source projection enabled", turns.rehydrateReq)
	}
	if turns.rehydrateReq.ProcessID != "process_root" {
		t.Fatalf("rehydrate process = %q, want process_root", turns.rehydrateReq.ProcessID)
	}
	if turns.rehydrateReq.GoalLeaseID != pending.GoalLeaseID {
		t.Fatalf("rehydrate goal lease = %q, want %q", turns.rehydrateReq.GoalLeaseID, pending.GoalLeaseID)
	}
	wantChildRuns := map[string]ChildRunBinding{
		"process_grandchild": {ProcessID: "process_grandchild", RunID: "run_grandchild", ParentRunID: "run_a"},
		"process_a":          {ProcessID: "process_a", RunID: "run_a", ParentRunID: "run_1"},
		"process_b":          {ProcessID: "process_b", RunID: "run_b", ParentRunID: "run_1"},
	}
	if len(turns.rehydrateReq.ChildRuns) != len(wantChildRuns) {
		t.Fatalf("rehydrate child Runs = %+v, want %+v", turns.rehydrateReq.ChildRuns, wantChildRuns)
	}
	for _, binding := range turns.rehydrateReq.ChildRuns {
		if want, ok := wantChildRuns[binding.ProcessID]; !ok || binding != want {
			t.Fatalf("rehydrate child Run binding = %+v, want one of %+v", binding, wantChildRuns)
		}
	}
}

func TestResumeRehydrateRestoresChildAdmissionBeforeAnyChildExists(t *testing.T) {
	createdAt := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	pending := testPendingInterrupt("item_1", "process_root", createdAt)
	pending.ProtocolProfile.ChildRuns = true
	pending.Continuations[0].ModelSelection = mustUseCaseSelection("openai", "model")
	sessions := &fakeRunSessions{
		sess: session.Session{ID: pending.SessionID, Cwd: "/work"},
		pending: map[string]interrupts.Pending{
			pending.RootRunID: pending,
		},
	}
	turns := &fakeTurnControl{
		prepareErr: ErrTurnNotLive,
		rehydrated: execution.TurnRef{
			SessionID: pending.SessionID,
			TurnID:    pending.TurnID,
		},
	}
	c := newUseCaseCoordinator(&fakeExecutor{}, turns, sessions, &fakeEffects{})

	result, err := c.Resume(t.Context(), ResumeCommand{
		RunID:              pending.RootRunID,
		CallerCapabilities: pending.ProtocolProfile,
		Responses: []ResumeResponse{{
			ItemID: "item_1",
			Kind:   ApprovalResponseKind,
			Approval: &ApprovalResponse{
				Approved: true,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	for range result.Events {
	}
	if len(pending.Continuations) != 1 {
		t.Fatalf("test fixture has %d continuations, want one root only", len(pending.Continuations))
	}
	if !turns.rehydrateReq.ChildRunAdmissionEnabled {
		t.Fatalf("rehydrate request = %+v, want frozen child policy restored", turns.rehydrateReq)
	}
}

func TestResumeRefusesIsolatedRunAfterSandboxProcessEnded(t *testing.T) {
	var operations []string
	sessions := &fakeRunSessions{
		sess: session.Session{ID: "ses_1", Cwd: "/work", Isolated: true},
		pending: map[string]interrupts.Pending{
			"run_1": testPendingInterrupt("item_1", "proc_1", time.Now().UTC()),
		},
		operations: &operations,
	}
	// The process that owned the sandbox copy is gone (Prepare reports the turn as
	// not live), so a rehydrate would run against the real project tree.
	turns := &fakeTurnControl{prepareErr: ErrTurnNotLive}
	c := newUseCaseCoordinator(&fakeExecutor{}, turns, sessions, &fakeEffects{})

	_, err := c.Resume(t.Context(), ResumeCommand{
		RunID: "run_1",
		CallerCapabilities: execution.RunProtocolProfile{
			InterruptKinds: []execution.InterruptKind{execution.ApprovalInterrupt},
		},
		Responses: []ResumeResponse{{
			ItemID: "item_1", Kind: ApprovalResponseKind,
			Approval: &ApprovalResponse{Approved: true},
		}},
	})
	if !errors.Is(err, ErrRunNotFound) || !errors.Is(err, ErrTurnStateLost) {
		t.Fatalf("Resume error = %v, want run not found wrapping turn state lost", err)
	}
	if turns.rehydrateReq.ProcessID != "" || len(turns.rehydrateReq.ChildRuns) != 0 {
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
		RunID:  "run_1",
		Kind:   execution.ApprovalInterrupt,
		Approval: &transcript.Approval{
			Tool: transcript.ToolInvocation{Name: "shell"},
		},
	}}
}

func testPendingInterrupt(itemID, processID string, runCreatedAt time.Time) interrupts.Pending {
	interruptValues := approvalInterrupt(itemID)
	return interrupts.Pending{
		RootRunID:  "run_1",
		SessionID:  "ses_1",
		TurnID:     "turn_1",
		Interrupts: interruptValues,
		ProtocolProfile: execution.RunProtocolProfile{
			InterruptKinds: []execution.InterruptKind{execution.ApprovalInterrupt},
		},
		Suspensions: []interrupts.SuspensionBinding{{
			InterruptItemID: itemID,
			ProcessID:       processID,
			SuspensionID:    "suspension_1",
		}},
		Continuations: []interrupts.Continuation{{
			RunID:        "run_1",
			ProcessID:    processID,
			RunCreatedAt: runCreatedAt,
		}},
		CreatedAt: runCreatedAt.Add(time.Second),
	}
}

func runForPending(pending interrupts.Pending) transcript.Run {
	root, _ := pending.RootContinuation()
	return runForContinuation(pending, root)
}

func runForContinuation(
	pending interrupts.Pending,
	continuation interrupts.Continuation,
) transcript.Run {
	goalLeaseID := ""
	if continuation.RunID == pending.RootRunID {
		goalLeaseID = pending.GoalLeaseID
	}
	return transcript.Run{
		ID:              continuation.RunID,
		SessionID:       pending.SessionID,
		SpawnedByItemID: continuation.Lineage.SpawnedByItemID,
		ParentRunID:     continuation.Lineage.ParentRunID,
		RootRunID:       continuation.Lineage.RootRunID,
		ModelSelection:  continuation.ModelSelection,
		GoalLeaseID:     goalLeaseID,
		State:           execution.Interrupted,
		Metrics:         continuation.Metrics,
		Limits:          continuation.Limits,
		ProtocolProfile: pending.ProtocolProfile,
		CreatedAt:       continuation.RunCreatedAt,
		MessageMark:     transcript.UnknownMessageMark,
	}
}

func TestCancelParkedRunUsesApplicationAdmission(t *testing.T) {
	var operations []string
	pending := testPendingInterrupt("item_1", "proc_1", time.Now().UTC())
	sessions := &fakeRunSessions{
		pending: map[string]interrupts.Pending{
			"run_1": pending,
		},
		operations: &operations,
	}
	turns := &fakeTurnControl{operations: &operations}
	c := NewCoordinator(Dependencies{
		Turns: turns, Sessions: sessions,
		Runs: &fakeRunProjection{runs: map[string]transcript.Run{
			"run_1": runForPending(pending),
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

func TestCancelRunningChildCommitsExactSubtreeBoundaryAndKeepsRootRunning(t *testing.T) {
	childRequest, childConfirmation := NewChildOpeningRequest(time.Now().UTC())
	rootSource := ExecutorSource{ProcessID: "process_root"}
	childSource := ExecutorSource{
		ProcessID:   "process_child",
		ParentID:    rootSource.ProcessID,
		SpawnCallID: "provider_child",
	}
	executor := &cancellableChildExecutor{
		rootSource:      rootSource,
		childSource:     childSource,
		request:         childRequest,
		confirmation:    childConfirmation,
		childOpened:     make(chan struct{}),
		cancelRequested: make(chan struct{}),
		finishRoot:      make(chan struct{}),
	}
	turns := &fakeTurnControl{}
	turns.cancelSubtree = func(ref execution.TurnRef, processID string) error {
		if ref != (execution.TurnRef{SessionID: "ses_1", TurnID: "turn_1"}) {
			return errors.New("subtree cancellation addressed the wrong turn")
		}
		if processID != childSource.ProcessID {
			return errors.New("subtree cancellation addressed the wrong process")
		}
		close(executor.cancelRequested)
		return nil
	}
	effects := &fakeEffects{}
	projection := &fakeRunProjection{runs: map[string]transcript.Run{
		"run_1": runForSegment(testSegment()),
	}}
	coordinator := NewCoordinator(Dependencies{
		Segments:     executor,
		Turns:        turns,
		Sessions:     &fakeRunSessions{},
		Effects:      effects,
		Runs:         projection,
		Admissions:   new(admission.Gate),
		Now:          func() time.Time { return time.Date(2026, 7, 30, 1, 2, 3, 0, time.UTC) },
		NewRunID:     func() string { return "run_child" },
		NewSegmentID: func() string { return "seg_child" },
	})
	stream, err := coordinator.openSegment(t.Context(), testSegment())
	if err != nil {
		t.Fatalf("open segment: %v", err)
	}
	streamDone := make(chan []Event, 1)
	go func() { streamDone <- collectEvents(stream) }()
	select {
	case <-executor.childOpened:
	case <-time.After(time.Second):
		t.Fatal("child opening was not committed")
	}

	openings := effects.openingSnapshot()
	if len(openings) != 2 || openings[1].Admit == nil {
		t.Fatalf("openings = %+v, want root and child", openings)
	}
	childDraft := *openings[1].Admit
	projection.runs[childDraft.RunID] = transcript.Run{
		ID:              childDraft.RunID,
		SessionID:       childDraft.SessionID,
		SpawnedByItemID: childDraft.SpawnedByItemID,
		ParentRunID:     childDraft.ParentRunID,
		RootRunID:       childDraft.RootRunID,
		State:           execution.Running,
		ActiveSegmentID: childDraft.SegmentID,
		ModelSelection:  childDraft.ModelSelection,
		Limits:          childDraft.Limits,
		ProtocolProfile: childDraft.ProtocolProfile,
		CreatedAt:       childDraft.CreatedAt,
		UpdatedAt:       childDraft.CreatedAt,
		MessageMark:     transcript.UnknownMessageMark,
	}

	result, err := coordinator.Cancel(t.Context(), CancelCommand{
		RunID:         childDraft.RunID,
		Reason:        "stop delegated work",
		AllowChildRun: true,
	})
	if err != nil {
		t.Fatalf("Cancel child: %v", err)
	}
	if result.Run.ID != childDraft.RunID ||
		result.Run.State != execution.Canceled ||
		result.Run.Outcome == nil ||
		*result.Run.Outcome != execution.OutcomeCanceled ||
		result.Run.Detail != "stop delegated work" {
		t.Fatalf("child result = %+v, want exact canceled terminal", result.Run)
	}
	if result.RootRun == nil ||
		result.RootRun.ID != "run_1" ||
		result.RootRun.State != execution.Running {
		t.Fatalf("root result = %+v, want still-running run_1", result.RootRun)
	}

	var (
		childTerminals int
		parentResults  int
	)
	for _, commit := range effects.commitSnapshot() {
		if commit.State == StateTerminalize && commit.RunID == childDraft.RunID {
			childTerminals++
		}
		for _, item := range commit.Items {
			if item.ID != childDraft.SpawnedByItemID {
				continue
			}
			if item.Status == transcript.ItemIncomplete &&
				item.Error != nil &&
				item.Error.Kind == transcript.ChildRunCanceledProblem &&
				item.Error.Scope == transcript.ToolProblem &&
				item.Error.Detail == "stop delegated work" {
				parentResults++
			}
		}
	}
	if childTerminals != 1 || parentResults != 1 {
		t.Fatalf(
			"child terminal commits=%d parent child_run_canceled results=%d, want 1/1",
			childTerminals,
			parentResults,
		)
	}
	if _, live := coordinator.registry.Get("run_1"); !live {
		t.Fatal("child cancellation stopped the root segment")
	}
	entry, live := coordinator.registry.Get("run_1")
	if !live || entry.handle == nil || entry.handle.hub == nil {
		t.Fatal("continued root has no event Journal")
	}
	afterCancellation := entry.handle.hub.Tail()
	cursorAfterCancellation := afterCancellation.HeadCursor
	afterCancellation.Cancel()
	if cursorAfterCancellation == "" {
		t.Fatal("child cancellation returned before the Journal established a cursor")
	}

	close(executor.finishRoot)
	select {
	case events := <-streamDone:
		var rootCompleted bool
		for _, event := range events {
			finished, ok := event.Payload.(SegmentFinished)
			if ok && event.RunID == "run_1" && finished.Run.State == execution.Completed {
				rootCompleted = true
			}
		}
		if !rootCompleted {
			t.Fatalf("root did not continue to its natural terminal: %+v", events)
		}
		replayed, err := entry.handle.hub.Replay(cursorAfterCancellation)
		if err != nil {
			t.Fatalf("replay after child cancellation: %v", err)
		}
		for _, event := range collectEvents(replayed.Events) {
			if event.RunID == childDraft.RunID {
				t.Fatalf(
					"canceled child published event after Cancel returned: %+v",
					event,
				)
			}
		}
	case <-time.After(time.Second):
		t.Fatal("root did not finish after child cancellation")
	}
}

func TestCancelParkedRunReportsTurnCleanupFailureAfterDurableCommit(t *testing.T) {
	cleanupErr := errors.New("turn cleanup failed")
	pending := testPendingInterrupt("item_1", "proc_1", time.Now().UTC())
	sessions := &fakeRunSessions{pending: map[string]interrupts.Pending{
		"run_1": pending,
	}}
	turns := &fakeTurnControl{cancelErr: cleanupErr}
	c := NewCoordinator(Dependencies{
		Turns: turns, Sessions: sessions,
		Runs: &fakeRunProjection{runs: map[string]transcript.Run{
			"run_1": runForPending(pending),
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
			"run_1": runForSegment(testSegment()),
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
	executor := &fakeExecutor{events: []ExecutorPayload{TurnEnd{
		Reason: execution.OutcomeCompleted,
	}}}
	effects := &fakeEffects{terminalStarted: terminalStarted, terminalRelease: releaseTerminal}
	c := NewCoordinator(Dependencies{
		Segments: executor, Turns: &fakeTurnControl{}, Sessions: &fakeRunSessions{}, Effects: effects,
		Runs: &fakeRunProjection{runs: map[string]transcript.Run{
			"run_1": runForSegment(testSegment()),
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
	executor := &fakeExecutor{events: []ExecutorPayload{
		ToolCallStart{
			CallID: "call_1", ToolName: "shell", Arguments: `{"command":"pwd"}`,
			SafetyClass: "write",
		},
		TreeInterrupted{Checkpoint: testExecutorCheckpoint(), Suspensions: []ProcessSuspension{{
			ProcessID: "process_root", SuspensionID: "suspension_1",
			Interrupt: Interrupt{
				Kind: execution.ApprovalInterrupt,
				Approval: &ApprovalPrompt{
					CallID: "call_1", ToolName: "shell", Arguments: `{"command":"pwd"}`,
					SafetyClass: "write",
				},
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
	spec := testSegment()
	spec.ProtocolProfile = execution.RunProtocolProfile{
		InterruptKinds: []execution.InterruptKind{execution.ApprovalInterrupt},
	}
	c := NewCoordinator(Dependencies{
		Segments: executor, Turns: turns, Sessions: sessions, Effects: effects,
		Runs: &fakeRunProjection{runs: map[string]transcript.Run{
			"run_1": runForSegment(spec),
		}},
		Admissions: new(admission.Gate),
	})
	stream, err := c.openSegment(t.Context(), spec)
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
	pending := testPendingInterrupt("item_1", "proc_1", time.Now().UTC())
	sessions := &fakeRunSessions{pending: map[string]interrupts.Pending{
		"run_1": pending,
	}}
	turns := &fakeTurnControl{cancelErr: ErrTurnNotLive}
	c := NewCoordinator(Dependencies{
		Turns: turns, Sessions: sessions,
		Runs: &fakeRunProjection{runs: map[string]transcript.Run{
			"run_1": runForPending(pending),
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
		RunID:             testRunID,
		ExpectedSegmentID: testSegmentID,
		Input: []transcript.ContentBlock{
			{Kind: transcript.TextContent, Text: "wait"},
			{Kind: transcript.ImageContent, Mime: "image/png", Data: "aW1hZ2U="},
		},
	}); err != nil {
		t.Fatalf("Steer: %v", err)
	}
	if len(turns.steered) != 1 || turns.steered[0] != (execution.TurnRef{SessionID: "ses_1", TurnID: "turn_1"}) {
		t.Fatalf("steered refs = %+v", turns.steered)
	}
	if len(turns.steerInput) != 2 ||
		turns.steerInput[0].Text != "wait" ||
		turns.steerInput[1].Kind != transcript.ImageContent {
		t.Fatalf("steer input = %+v", turns.steerInput)
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
