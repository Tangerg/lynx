package runs

import (
	"context"
	"errors"
	"iter"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/admission"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	corechat "github.com/Tangerg/lynx/core/chat"
)

type fakeRunSessions struct {
	sess session.Session
	// active is the Session's non-terminal Run, when it has one. The zero value is a
	// Session free to start.
	active        *transcript.Run
	createdTitle  string
	pending       map[string]Pending
	canceledRunID string
	cancelReason  string
	canceledAt    time.Time
	lostRunID     string
	lostAt        time.Time
	lostErr       error
	operations    *[]string
}

type completeTestSessionPorts interface {
	SessionReader
	SessionCreator
	ActiveRunReader
	PendingInterruptReader
	RunTerminationCommitter
}

func testSessionPorts(ports completeTestSessionPorts) SessionPorts {
	return SessionPorts{
		Reader:       ports,
		Creator:      ports,
		ActiveRuns:   ports,
		Interrupts:   ports,
		Terminations: ports,
	}
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
	f.sess = session.Session{ID: "ses_created", CWD: cwd}
	return f.sess, nil
}

func (f *fakeRunSessions) PrepareScheduled(_ context.Context, id, title, cwd string) (session.Session, error) {
	f.createdTitle = title
	f.sess = session.Session{ID: id, CWD: cwd}
	return f.sess, nil
}

func (f *fakeRunSessions) ListOpenInterrupts(_ context.Context, sessionID string) ([]Pending, error) {
	var out []Pending
	for _, pending := range f.pending {
		if pending.SessionID == sessionID {
			out = append(out, pending)
		}
	}
	return out, nil
}

func (f *fakeRunSessions) LookupOpenInterrupt(_ context.Context, runID string) (Pending, bool, error) {
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
	outcome := run.OutcomeCanceled
	return transcript.Run{
		ID: runID, SessionID: sessionID, State: run.Canceled,
		Outcome: &outcome, Detail: reason, FinishedAt: finishedAt,
	}, nil
}

func (f *fakeRunSessions) ApplyRunLost(_ context.Context, _ string, runID string, finishedAt time.Time) error {
	if f.operations != nil {
		*f.operations = append(*f.operations, "durable.lost")
	}
	if f.lostErr != nil {
		return f.lostErr
	}
	f.lostRunID = runID
	f.lostAt = finishedAt
	delete(f.pending, runID)
	return nil
}

type fakeExecutionPorts struct {
	validated         RootExecutionStart
	started           RootExecutionStart
	startRef          ExecutorRef
	prepared          ExecutorRef
	prepareErr        error
	rehydrated        ExecutorRef
	continuation      WaitingContinuation
	rehydrateErr      error
	resumeCheck       func()
	activateCheck     func()
	activated         bool
	resumed           bool
	released          []ExecutorRef
	canceledTrees     []ExecutorRef
	steered           []ExecutorRef
	steerInput        []transcript.ContentBlock
	operations        *[]string
	releaseErr        error
	requestRootCancel func()
	cancelTreeErr     error
	cancelSubtree     func(ExecutorRef, string, string) error
	prepareWaiting    func(WaitingSubtreeCancellationRequest) (PreparedWaitingSubtreeCancellation, error)
	restoreWaiting    []WaitingContinuation
	restoreErr        error
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

func (f *fakeExecutionPorts) ValidateRootStart(req RootExecutionStart) error {
	f.validated = req
	return nil
}

func (f *fakeExecutionPorts) StageRoot(_ context.Context, req RootExecutionStart) (ExecutorRef, error) {
	f.started = req
	return f.startRef, nil
}

func (f *fakeExecutionPorts) BeginRoot(context.Context, ExecutorRef) error {
	if f.activateCheck != nil {
		f.activateCheck()
	}
	f.activated = true
	return nil
}

func (f *fakeExecutionPorts) StageContinuation(_ context.Context, continuation WaitingContinuation) (ExecutorRef, error) {
	f.continuation = continuation
	if f.prepareErr != nil {
		if !errors.Is(f.prepareErr, ErrExecutorNotLive) {
			return ExecutorRef{}, f.prepareErr
		}
	}
	if f.rehydrateErr != nil {
		return ExecutorRef{}, f.rehydrateErr
	}
	if f.prepared != (ExecutorRef{}) {
		return f.prepared, nil
	}
	if f.rehydrated != (ExecutorRef{}) {
		return f.rehydrated, nil
	}
	return ExecutorRef{SessionID: continuation.SessionID, ExecutorID: continuation.ExecutorID}, nil
}

func (f *fakeExecutionPorts) RestoreWaitingExecution(
	_ context.Context,
	continuation WaitingContinuation,
) (ExecutorRef, error) {
	f.restoreWaiting = append(f.restoreWaiting, continuation)
	if f.operations != nil {
		*f.operations = append(*f.operations, "executor.restore_waiting")
	}
	if f.restoreErr != nil {
		return ExecutorRef{}, f.restoreErr
	}
	return ExecutorRef{SessionID: continuation.SessionID, ExecutorID: continuation.ExecutorID}, nil
}

func testChildRunBindings(members []WaitingMember) []ChildRunBinding {
	bindings := make([]ChildRunBinding, 0, len(members)-1)
	for _, member := range members {
		if member.ParentRunID == "" {
			continue
		}
		bindings = append(bindings, ChildRunBinding{
			MemberID: member.MemberID, RunID: member.RunID, ParentRunID: member.ParentRunID,
		})
	}
	return bindings
}

func (f *fakeExecutionPorts) BeginContinuation(
	context.Context,
	ExecutorRef,
	[]InterruptAnswer,
	[]interrupt.Kind,
) error {
	if f.resumeCheck != nil {
		f.resumeCheck()
	}
	f.resumed = true
	return nil
}

func (f *fakeExecutionPorts) Release(_ context.Context, ref ExecutorRef) error {
	if f.operations != nil {
		*f.operations = append(*f.operations, "executor.release")
	}
	f.released = append(f.released, ref)
	return f.releaseErr
}

func (f *fakeExecutionPorts) RequestRootCancellation(context.Context, ExecutorRef, string) error {
	if f.requestRootCancel != nil {
		f.requestRootCancel()
	}
	return nil
}

func (f *fakeExecutionPorts) CancelRunningSubtree(
	_ context.Context,
	ref ExecutorRef,
	memberID string,
	reason string,
) error {
	if f.operations != nil {
		*f.operations = append(*f.operations, "run.cancel_subtree:"+memberID)
	}
	f.canceledTrees = append(f.canceledTrees, ref)
	if f.cancelSubtree != nil {
		return f.cancelSubtree(ref, memberID, reason)
	}
	return f.cancelTreeErr
}

func (f *fakeExecutionPorts) PrepareWaitingSubtreeCancellation(
	_ context.Context,
	request WaitingSubtreeCancellationRequest,
) (PreparedWaitingSubtreeCancellation, error) {
	if f.prepareWaiting == nil {
		return PreparedWaitingSubtreeCancellation{}, errors.New("fake execution control: waiting subtree cancellation is not configured")
	}
	return f.prepareWaiting(request)
}

func (f *fakeExecutionPorts) SubmitSteer(_ context.Context, ref ExecutorRef, input []transcript.ContentBlock) error {
	f.steered = append(f.steered, ref)
	f.steerInput = append([]transcript.ContentBlock(nil), input...)
	return nil
}

func newUseCaseCoordinator(exec ExecutionObserver, control *fakeExecutionPorts, sessions completeTestSessionPorts, effects completeTestProjectionPorts) *Coordinator {
	if executor, ok := exec.(*fakeExecutor); ok && control.requestRootCancel == nil {
		control.requestRootCancel = executor.requestRootCancellation
	}
	freshCreatedAt := time.Date(2026, 7, 13, 1, 2, 3, 0, time.UTC)
	projection := &fakeRunProjection{runs: map[string]transcript.Run{
		"run_1": runForSegment(testSegment()),
		"run_new": runForSegment(segmentSpec{
			RunID: "run_new", SegmentID: "seg_new", SessionID: "ses_1",
			ExecutorID: "turn_1", CreatedAt: freshCreatedAt,
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
		RootStarts:                         control,
		Observations:                       exec,
		Releases:                           control,
		RootCancellation:                   control,
		Conversation:                       emptyConversationReader{},
		Continuation:                       control,
		WaitingRestorer:                    control,
		Steering:                           control,
		RunningSubtreeCanceler:             control,
		WaitingSubtreeCancellationPreparer: control,
		Session:                            testSessionPorts(sessions),
		Projection:                         testProjectionPorts(effects),
		Runs:                               projection,
		Now:                                func() time.Time { return time.Date(2026, 7, 13, 1, 2, 3, 0, time.UTC) },
		NewRunID:                           func() string { return "run_new" },
		NewSegmentID:                       func() string { return "seg_new" },
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
	sessions := &fakeRunSessions{sess: session.Session{ID: "ses_1", CWD: "/work"}}
	c := newUseCaseCoordinator(&fakeExecutor{}, &fakeExecutionPorts{}, sessions, &fakeEffects{})
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
	sessions := &fakeRunSessions{sess: session.Session{ID: "ses_1", CWD: "/work"}}
	control := &fakeExecutionPorts{startRef: ExecutorRef{SessionID: "ses_1", ExecutorID: "turn_1"}}
	activatedAfterOpening := false
	control.activateCheck = func() { activatedAfterOpening = effects.opening().Admit != nil }
	c := newUseCaseCoordinator(exec, control, sessions, effects)

	result, err := c.Start(context.Background(), StartCommand{
		SessionID:      "ses_1",
		ModelSelection: mustUseCaseSelection("provider", "model"),
		Limits:         run.Limits{MaxTotalTokens: 16_384, MaxSteps: 12, MaxBudgetUSD: 3.5},
		Capabilities:   run.Capabilities{ChildRuns: true},
		Input:          []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "hello"}},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	consumeEvents(result.Events)
	if result.RunID != "run_new" || result.SegmentID != "seg_new" || result.SessionID != "ses_1" {
		t.Fatalf("result = %+v", result)
	}
	if control.started.SessionID != "ses_1" || control.started.CWD != "/work" || control.started.WorkspaceCWD != "/work" {
		t.Fatalf("started execution = %+v", control.started)
	}
	wantLimits := run.Limits{MaxTotalTokens: 16_384, MaxSteps: 12, MaxBudgetUSD: 3.5}
	if control.started.Limits != wantLimits {
		t.Fatalf("executor limits = %+v, want %+v", control.started.Limits, wantLimits)
	}
	if !control.validated.ChildRunAdmissionEnabled || !control.started.ChildRunAdmissionEnabled {
		t.Fatalf("child admission policy did not reach the executor: validated=%+v started=%+v", control.validated, control.started)
	}
	if !control.activated || !activatedAfterOpening {
		t.Fatalf("activated=%v activatedAfterOpening=%v", control.activated, activatedAfterOpening)
	}
	if opening := effects.opening(); opening.Admit == nil || opening.Admit.RunID != "run_new" {
		t.Fatalf("opening = %+v, want fresh run admission", opening)
	} else if opening.Admit.Limits != wantLimits {
		t.Fatalf("opening limits = %+v, want %+v", opening.Admit.Limits, wantLimits)
	} else if opening.SessionModel == nil || opening.SessionModel.SessionID != "ses_1" || opening.SessionModel.Model != "model" {
		t.Fatalf("opening session model = %+v, want ses_1/model", opening.SessionModel)
	}
}

func TestStartSeedsExecutorFromConversationAndCurrentUserMessage(t *testing.T) {
	exec := &fakeExecutor{}
	effects := &fakeEffects{}
	sessions := &fakeRunSessions{sess: session.Session{ID: "ses_1", CWD: "/work"}}
	control := &fakeExecutionPorts{startRef: ExecutorRef{SessionID: "ses_1", ExecutorID: "turn_1"}}
	c := newUseCaseCoordinator(exec, control, sessions, effects)
	c.conversation = staticConversationReader{messages: []corechat.Message{
		corechat.NewUserMessage(corechat.NewTextPart("earlier question")),
		corechat.NewAssistantMessage(corechat.NewTextPart("earlier answer")),
	}}

	result, err := c.Start(t.Context(), StartCommand{
		SessionID: "ses_1",
		Input:     []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "current question"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	consumeEvents(result.Events)
	context := control.started.WorkingContext
	if len(context) != 3 || context[0].Text() != "earlier question" ||
		context[1].Text() != "earlier answer" || context[2].Text() != "current question" ||
		context[2].Role != corechat.RoleUser {
		t.Fatalf("working context = %#v", context)
	}
}

type staticConversationReader struct{ messages []corechat.Message }

func (reader staticConversationReader) Read(context.Context, string) ([]corechat.Message, error) {
	messages := make([]corechat.Message, len(reader.messages))
	for index := range reader.messages {
		messages[index] = reader.messages[index].Clone()
	}
	return messages, nil
}

func TestStartSeparatesIsolatedExecutionDirFromPersistentWorkspace(t *testing.T) {
	exec := &fakeExecutor{}
	effects := &fakeEffects{}
	sessions := &fakeRunSessions{sess: session.Session{ID: "ses_1", CWD: "/work", Isolated: true}}
	control := &fakeExecutionPorts{startRef: ExecutorRef{SessionID: "ses_1", ExecutorID: "turn_1"}}
	c := newUseCaseCoordinator(exec, control, sessions, effects)
	c.isolation = &stubIsolation{path: "/sandbox/copy"}

	result, err := c.Start(t.Context(), StartCommand{
		SessionID: "ses_1",
		Input:     []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "hello"}},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	consumeEvents(result.Events)
	if control.started.CWD != "/sandbox/copy" || control.started.WorkspaceCWD != "/work" || !control.started.Isolated {
		t.Fatalf("started execution scope = %+v", control.started)
	}
}

func TestStartDoesNotActivateRejectedAdmission(t *testing.T) {
	exec := &fakeExecutor{}
	openingErr := errors.New("opening commit failed")
	effects := &fakeEffects{openingErr: openingErr}
	sessions := &fakeRunSessions{sess: session.Session{ID: "ses_1", CWD: "/work"}}
	control := &fakeExecutionPorts{startRef: ExecutorRef{SessionID: "ses_1", ExecutorID: "turn_1"}}
	c := newUseCaseCoordinator(exec, control, sessions, effects)

	_, err := c.Start(t.Context(), StartCommand{SessionID: "ses_1", Input: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "hello"}}})
	if !errors.Is(err, openingErr) {
		t.Fatalf("Start error = %v, want opening failure", err)
	}
	if control.activated {
		t.Fatal("rejected admission activated the prepared execution")
	}
	if len(control.released) != 1 {
		t.Fatalf("staged execution releases = %d, want 1", len(control.released))
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
			control := &fakeExecutionPorts{}
			effects := &fakeEffects{}
			sessions := &fakeRunSessions{sess: session.Session{ID: "ses_existing", CWD: "/work"}}
			command.Input = []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "hello"}}
			_, err := newUseCaseCoordinator(exec, control, sessions, effects).Start(t.Context(), command)
			if !errors.Is(err, ErrInvalidScheduledStart) {
				t.Fatalf("Start error = %v, want ErrInvalidScheduledStart", err)
			}
			if control.started.SessionID != "" || len(effects.openings) != 0 {
				t.Fatalf("partial scheduled identity reached side effects: execution=%+v openings=%d", control.started, len(effects.openings))
			}
		})
	}
}

func TestFastStartReleaseCannotCrossTerminalMaintenance(t *testing.T) {
	finishStarted := make(chan struct{}, 1)
	releaseFinish := make(chan struct{})
	sessions := &fakeRunSessions{
		sess: session.Session{ID: "ses_1", CWD: "/work"},
	}
	effects := &fakeEffects{finishStarted: finishStarted, finishRelease: releaseFinish}
	c := newUseCaseCoordinator(
		&fakeExecutor{events: []ExecutorPayload{SegmentEnded{Reason: run.OutcomeCompleted}}},
		&fakeExecutionPorts{startRef: ExecutorRef{SessionID: "ses_1", ExecutorID: "turn_1"}},
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
	consumeEvents(outcome.result.Events)
	requireCoordinatorShutdown(t, c)
	if hasActiveSession(c, "ses_1") {
		t.Fatal("terminal maintenance did not release its claim")
	}
}

func TestStartRejectsForeignExecutorIdentityAndReleasesIt(t *testing.T) {
	exec := &fakeExecutor{}
	effects := &fakeEffects{}
	sessions := &fakeRunSessions{sess: session.Session{ID: "ses_1", CWD: "/work"}}
	control := &fakeExecutionPorts{startRef: ExecutorRef{SessionID: "ses_foreign", ExecutorID: "turn_1"}}
	c := newUseCaseCoordinator(exec, control, sessions, effects)

	_, err := c.Start(context.Background(), StartCommand{SessionID: "ses_1", Input: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "hello"}}})
	if !errors.Is(err, ErrInvalidExecutorRef) {
		t.Fatalf("Start error = %v, want ErrInvalidExecutorRef", err)
	}
	if len(control.released) != 1 || control.released[0] != control.startRef {
		t.Fatalf("canceled control = %+v, want invalid started execution", control.released)
	}
	if _, ok := c.registry.Get("run_new"); len(effects.openings) != 0 || ok {
		t.Fatal("invalid execution identity reached Run admission")
	}
}

func TestResumeCommitsOpeningBeforeActivation(t *testing.T) {
	createdAt := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	effects := &fakeEffects{}
	sessions := &fakeRunSessions{
		sess: session.Session{ID: "ses_1", CWD: "/work"},
		pending: map[string]Pending{
			"run_1": testApprovalPending("member_1", createdAt),
		},
	}
	control := &fakeExecutionPorts{prepared: ExecutorRef{SessionID: "ses_1", ExecutorID: "turn_1"}}
	activatedAfterOpening := false
	control.resumeCheck = func() { activatedAfterOpening = effects.opening().Resume != nil }
	c := newUseCaseCoordinator(&fakeExecutor{}, control, sessions, effects)

	result, err := c.Resume(context.Background(), ResumeCommand{
		RunID: "run_1",
		CallerCapabilities: run.Capabilities{
			InterruptKinds: []interrupt.Kind{interrupt.Approval},
		},
		Responses: []ResumeResponse{{
			ItemID: "item_1", Kind: ApprovalResponseKind,
			Approval: &ApprovalResponse{Approved: true},
		}},
	})
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	consumeEvents(result.Events)
	if !control.resumed || !activatedAfterOpening {
		t.Fatalf("resumed=%v activatedAfterOpening=%v", control.resumed, activatedAfterOpening)
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
	pending := testApprovalPending("member_root", createdAt)
	sessions := &fakeRunSessions{
		sess: session.Session{ID: pending.SessionID, CWD: "/work"},
		pending: map[string]Pending{
			pending.RootRunID: pending,
		},
	}
	control := &fakeExecutionPorts{prepared: ExecutorRef{
		SessionID:  pending.SessionID,
		ExecutorID: pending.ExecutorID,
	}}
	effects := &fakeEffects{}
	coordinator := newUseCaseCoordinator(&fakeExecutor{}, control, sessions, effects)
	contradictory := runForPending(pending)
	contradictory.Metrics.Steps++
	coordinator.runs = &fakeRunProjection{runs: map[string]transcript.Run{
		pending.RootRunID: contradictory,
	}}

	_, err := coordinator.Resume(t.Context(), ResumeCommand{
		RunID:              pending.RootRunID,
		CallerCapabilities: pending.Capabilities,
		Responses: []ResumeResponse{{
			ItemID: "item_1", Kind: ApprovalResponseKind,
			Approval: &ApprovalResponse{Approved: true},
		}},
	})
	if err == nil {
		t.Fatal("Resume accepted cumulative metrics that differ from the durable Run")
	}
	if control.resumed || control.continuation.Checkpoint.RootMemberID != "" || len(effects.openings) != 0 {
		t.Fatalf("contradictory continuation reached executor/effects: control=%+v openings=%d", control, len(effects.openings))
	}
	if _, found := sessions.pending[pending.RootRunID]; !found {
		t.Fatal("failed validation consumed the open Pending set")
	}
}

func TestResumeAndRootCancelShareOneApplicationAdmissionBoundary(t *testing.T) {
	createdAt := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	pending := testApprovalPending("member_1", createdAt)
	sessions := &fakeRunSessions{
		sess: session.Session{ID: "ses_1", CWD: "/work"},
		pending: map[string]Pending{
			"run_1": pending,
		},
	}
	control := &fakeExecutionPorts{
		prepared: ExecutorRef{SessionID: "ses_1", ExecutorID: "turn_1"},
	}
	openingStarted := make(chan struct{}, 1)
	releaseOpening := make(chan struct{})
	effects := &blockingOpeningEffects{
		fakeEffects: &fakeEffects{},
		started:     openingStarted,
		release:     releaseOpening,
	}
	c := newUseCaseCoordinator(&fakeExecutor{}, control, sessions, effects)
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
			CallerCapabilities: run.Capabilities{
				InterruptKinds: []interrupt.Kind{interrupt.Approval},
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
	consumeEvents(outcome.result.Events)
	if !control.resumed {
		t.Fatal("winning resume did not activate its continuation")
	}
}

// TestResumeWithInputCommitsTheUserItemWithTheContinuation is the atomic half of
// "approve, and also do this differently". Before resume could carry input, that was
// two calls — resume then steer — with a window between them where the model could
// finish the tool round before the instruction ever arrived.
//
// The user Item rides the SAME opening write-set as the continuation, so either both
// landed or neither did, and the response names the item only when there is one: that
// iff is a cross-shape rule no schema keyword can state, so it is held here.
func TestResumeWithInputCommitsTheUserItemWithTheContinuation(t *testing.T) {
	createdAt := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	newResumeCase := func() (*fakeEffects, *Coordinator) {
		effects := &fakeEffects{}
		sessions := &fakeRunSessions{
			sess: session.Session{ID: "ses_1", CWD: "/work"},
			pending: map[string]Pending{
				"run_1": testApprovalPending("member_1", createdAt),
			},
		}
		control := &fakeExecutionPorts{prepared: ExecutorRef{SessionID: "ses_1", ExecutorID: "turn_1"}}
		return effects, newUseCaseCoordinator(&fakeExecutor{}, control, sessions, effects)
	}
	approve := []ResumeResponse{{
		ItemID: "item_1", Kind: ApprovalResponseKind,
		Approval: &ApprovalResponse{Approved: true},
	}}

	effects, c := newResumeCase()
	withInput, err := c.Resume(context.Background(), ResumeCommand{
		RunID: "run_1", Responses: approve,
		CallerCapabilities: run.Capabilities{
			InterruptKinds: []interrupt.Kind{interrupt.Approval},
		},
		Input: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "also skip the tests"}},
	})
	if err != nil {
		t.Fatalf("Resume with input: %v", err)
	}
	consumeEvents(withInput.Events)
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
		CallerCapabilities: run.Capabilities{
			InterruptKinds: []interrupt.Kind{interrupt.Approval},
		},
	})
	if err != nil {
		t.Fatalf("Resume without input: %v", err)
	}
	consumeEvents(without.Events)
	if without.UserItemID != "" {
		t.Fatalf("userItemId = %q on a resume that opened no user input", without.UserItemID)
	}
}

func TestResumeRecoversLostExecutorStateBeforeReturning(t *testing.T) {
	var operations []string
	sessions := &fakeRunSessions{
		sess: session.Session{ID: "ses_1", CWD: "/work"},
		pending: map[string]Pending{
			"run_1": testApprovalPending("member_1", time.Now().UTC()),
		},
		operations: &operations,
	}
	control := &fakeExecutionPorts{
		prepareErr:   ErrExecutorNotLive,
		rehydrateErr: ErrExecutorStateLost,
	}
	c := newUseCaseCoordinator(&fakeExecutor{}, control, sessions, &fakeEffects{})

	_, err := c.Resume(t.Context(), ResumeCommand{
		RunID: "run_1",
		CallerCapabilities: run.Capabilities{
			InterruptKinds: []interrupt.Kind{interrupt.Approval},
		},
		Responses: []ResumeResponse{{
			ItemID: "item_1", Kind: ApprovalResponseKind,
			Approval: &ApprovalResponse{Approved: true},
		}},
	})
	if !errors.Is(err, ErrRunNotFound) || !errors.Is(err, ErrExecutorStateLost) {
		t.Fatalf("Resume error = %v, want Run not found wrapping executor state lost", err)
	}
	if sessions.lostRunID != "run_1" || sessions.lostAt.IsZero() {
		t.Fatalf("lost recovery = %q/%v, want run_1 and terminal time", sessions.lostRunID, sessions.lostAt)
	}
	if len(operations) != 1 || operations[0] != "durable.lost" {
		t.Fatalf("operations = %v, want one durable lost commit", operations)
	}
	if control.continuation.Checkpoint.Scope.CWD != "/work" {
		t.Fatalf("continuation cwd = %q, want /work", control.continuation.Checkpoint.Scope.CWD)
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

func TestResumeOpeningFailureMarksClaimedRunLostBeforeReleasingTree(t *testing.T) {
	var operations []string
	openingErr := errors.New("opening commit failed")
	sessions := &fakeRunSessions{
		sess: session.Session{ID: "ses_1", CWD: "/work"},
		pending: map[string]Pending{
			"run_1": testApprovalPending("member_1", time.Now().UTC()),
		},
		operations: &operations,
	}
	control := &fakeExecutionPorts{
		prepared:   ExecutorRef{SessionID: "ses_1", ExecutorID: "turn_1"},
		operations: &operations,
	}
	coordinator := newUseCaseCoordinator(
		&fakeExecutor{},
		control,
		sessions,
		&fakeEffects{openingErr: openingErr},
	)

	_, err := coordinator.Resume(t.Context(), ResumeCommand{
		RunID: "run_1",
		CallerCapabilities: run.Capabilities{
			InterruptKinds: []interrupt.Kind{interrupt.Approval},
		},
		Responses: []ResumeResponse{{
			ItemID: "item_1", Kind: ApprovalResponseKind,
			Approval: &ApprovalResponse{Approved: true},
		}},
	})
	if !errors.Is(err, ErrRunNotFound) || !errors.Is(err, openingErr) {
		t.Fatalf("Resume error = %v, want Run not found wrapping opening failure", err)
	}
	if control.resumed {
		t.Fatal("failed opening submitted the semantic answer")
	}
	if sessions.lostRunID != "run_1" || len(control.released) != 1 {
		t.Fatalf("lost Run/released tree = %q/%+v, want run_1 and one release", sessions.lostRunID, control.released)
	}
	if !slices.Equal(operations, []string{"durable.lost", "executor.release"}) {
		t.Fatalf("cleanup operations = %v, want durable.lost then executor.release", operations)
	}
}

func TestResumeRejectsClaimResultDriftBeforeStagingAndMarksRunLost(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*ClaimedResume)
	}{
		{
			name: "Pending",
			mutate: func(claimed *ClaimedResume) {
				claimed.Pending.ExecutorID = "exec_foreign"
			},
		},
		{
			name: "answer",
			mutate: func(claimed *ClaimedResume) {
				claimed.Answers[0].Resolution.Approved = false
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var operations []string
			sessions := &fakeRunSessions{
				sess: session.Session{ID: "ses_1", CWD: "/work"},
				pending: map[string]Pending{
					"run_1": testApprovalPending("member_1", time.Now().UTC()),
				},
				operations: &operations,
			}
			control := &fakeExecutionPorts{
				prepared: ExecutorRef{SessionID: "ses_1", ExecutorID: "turn_1"},
			}
			coordinator := newUseCaseCoordinator(
				&fakeExecutor{},
				control,
				sessions,
				&fakeEffects{mutateClaim: test.mutate},
			)

			_, err := coordinator.Resume(t.Context(), ResumeCommand{
				RunID: "run_1",
				CallerCapabilities: run.Capabilities{
					InterruptKinds: []interrupt.Kind{interrupt.Approval},
				},
				Responses: []ResumeResponse{{
					ItemID: "item_1", Kind: ApprovalResponseKind,
					Approval: &ApprovalResponse{Approved: true},
				}},
			})
			if !errors.Is(err, ErrRunNotFound) {
				t.Fatalf("Resume error = %v, want Run not found", err)
			}
			if control.continuation.ExecutorID != "" || control.resumed || len(control.released) != 0 {
				t.Fatalf("claim drift reached executor: %+v", control)
			}
			if sessions.lostRunID != "run_1" || !slices.Equal(operations, []string{"durable.lost"}) {
				t.Fatalf("lost recovery = %q operations=%v", sessions.lostRunID, operations)
			}
		})
	}
}

func TestResumeOpeningFailureKeepsTreeWhenRunLostCommitFails(t *testing.T) {
	var operations []string
	openingErr := errors.New("opening commit failed")
	lostErr := errors.New("RunLost commit failed")
	sessions := &fakeRunSessions{
		sess: session.Session{ID: "ses_1", CWD: "/work"},
		pending: map[string]Pending{
			"run_1": testApprovalPending("member_1", time.Now().UTC()),
		},
		lostErr:    lostErr,
		operations: &operations,
	}
	control := &fakeExecutionPorts{
		prepared:   ExecutorRef{SessionID: "ses_1", ExecutorID: "turn_1"},
		operations: &operations,
	}
	coordinator := newUseCaseCoordinator(
		&fakeExecutor{},
		control,
		sessions,
		&fakeEffects{openingErr: openingErr},
	)

	_, err := coordinator.Resume(t.Context(), ResumeCommand{
		RunID: "run_1",
		CallerCapabilities: run.Capabilities{
			InterruptKinds: []interrupt.Kind{interrupt.Approval},
		},
		Responses: []ResumeResponse{{
			ItemID: "item_1", Kind: ApprovalResponseKind,
			Approval: &ApprovalResponse{Approved: true},
		}},
	})
	if !errors.Is(err, openingErr) || !errors.Is(err, lostErr) {
		t.Fatalf("Resume error = %v, want opening and RunLost failures", err)
	}
	if len(control.released) != 0 {
		t.Fatalf("released tree = %+v before durable RunLost", control.released)
	}
	if !slices.Equal(operations, []string{"durable.lost"}) {
		t.Fatalf("cleanup operations = %v, want only failed durable attempt", operations)
	}
	if _, found := sessions.pending["run_1"]; !found {
		t.Fatal("failed RunLost commit removed the durable interrupt")
	}
}

func TestResumeOpeningFailureReportsReleaseAfterDurableRunLost(t *testing.T) {
	var operations []string
	openingErr := errors.New("opening commit failed")
	releaseErr := errors.New("tree release failed")
	sessions := &fakeRunSessions{
		sess: session.Session{ID: "ses_1", CWD: "/work"},
		pending: map[string]Pending{
			"run_1": testApprovalPending("member_1", time.Now().UTC()),
		},
		operations: &operations,
	}
	control := &fakeExecutionPorts{
		prepared:   ExecutorRef{SessionID: "ses_1", ExecutorID: "turn_1"},
		operations: &operations,
		releaseErr: releaseErr,
	}
	coordinator := newUseCaseCoordinator(
		&fakeExecutor{},
		control,
		sessions,
		&fakeEffects{openingErr: openingErr},
	)

	_, err := coordinator.Resume(t.Context(), ResumeCommand{
		RunID: "run_1",
		CallerCapabilities: run.Capabilities{
			InterruptKinds: []interrupt.Kind{interrupt.Approval},
		},
		Responses: []ResumeResponse{{
			ItemID: "item_1", Kind: ApprovalResponseKind,
			Approval: &ApprovalResponse{Approved: true},
		}},
	})
	if !errors.Is(err, ErrRunNotFound) || !errors.Is(err, openingErr) || !errors.Is(err, releaseErr) {
		t.Fatalf("Resume error = %v, want lost opening plus release failure", err)
	}
	if sessions.lostRunID != "run_1" || len(control.released) != 1 {
		t.Fatalf("lost Run/release attempts = %q/%+v, want run_1 and one attempt", sessions.lostRunID, control.released)
	}
	if !slices.Equal(operations, []string{"durable.lost", "executor.release"}) {
		t.Fatalf("cleanup operations = %v, want durable.lost then executor.release", operations)
	}
}

func TestResumeRehydrateRestoresChildSourceProjection(t *testing.T) {
	createdAt := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	pending := resumedTreePending(createdAt)
	pending.Capabilities.ChildRuns = true
	pending.GoalLeaseID = "goal-lease-1"
	sessions := &fakeRunSessions{
		sess: session.Session{ID: pending.SessionID, CWD: "/work"},
		pending: map[string]Pending{
			pending.RootRunID: pending,
		},
	}
	control := &fakeExecutionPorts{
		prepareErr: ErrExecutorNotLive,
		rehydrated: ExecutorRef{
			SessionID:  pending.SessionID,
			ExecutorID: pending.ExecutorID,
		},
	}
	c := newUseCaseCoordinator(&fakeExecutor{}, control, sessions, &fakeEffects{})
	segmentIDs := []string{"segment_root", "segment_grandchild", "segment_a", "segment_b"}
	c.newSegmentID = func() string {
		next := segmentIDs[0]
		segmentIDs = segmentIDs[1:]
		return next
	}
	result, err := c.Resume(t.Context(), ResumeCommand{
		RunID:              pending.RootRunID,
		CallerCapabilities: pending.Capabilities,
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
	consumeEvents(result.Events)
	if !control.continuation.ChildRunAdmissionEnabled {
		t.Fatalf("rehydrate request = %+v, want child member projection enabled", control.continuation)
	}
	if control.continuation.Checkpoint.RootMemberID != "member_root" {
		t.Fatalf("continuation member = %q, want member_root", control.continuation.Checkpoint.RootMemberID)
	}
	if control.continuation.Checkpoint.Scope.GoalLeaseID != pending.GoalLeaseID {
		t.Fatalf("continuation goal lease = %q, want %q", control.continuation.Checkpoint.Scope.GoalLeaseID, pending.GoalLeaseID)
	}
	wantChildRuns := map[string]ChildRunBinding{
		"member_grandchild": {MemberID: "member_grandchild", RunID: "run_grandchild", ParentRunID: "run_a"},
		"member_a":          {MemberID: "member_a", RunID: "run_a", ParentRunID: "run_1"},
		"member_b":          {MemberID: "member_b", RunID: "run_b", ParentRunID: "run_1"},
	}
	childRuns := testChildRunBindings(control.continuation.Members)
	if len(childRuns) != len(wantChildRuns) {
		t.Fatalf("continuation child Runs = %+v, want %+v", childRuns, wantChildRuns)
	}
	for _, binding := range childRuns {
		if want, ok := wantChildRuns[binding.MemberID]; !ok || binding != want {
			t.Fatalf("continuation child Run binding = %+v, want one of %+v", binding, wantChildRuns)
		}
	}
}

func TestResumeRehydrateRestoresChildAdmissionBeforeAnyChildExists(t *testing.T) {
	createdAt := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	pending := testApprovalPending("member_root", createdAt)
	pending.Capabilities.ChildRuns = true
	pending.Continuations[0].ModelSelection = mustUseCaseSelection("openai", "model")
	sessions := &fakeRunSessions{
		sess: session.Session{ID: pending.SessionID, CWD: "/work"},
		pending: map[string]Pending{
			pending.RootRunID: pending,
		},
	}
	control := &fakeExecutionPorts{
		prepareErr: ErrExecutorNotLive,
		rehydrated: ExecutorRef{
			SessionID:  pending.SessionID,
			ExecutorID: pending.ExecutorID,
		},
	}
	c := newUseCaseCoordinator(&fakeExecutor{}, control, sessions, &fakeEffects{})

	result, err := c.Resume(t.Context(), ResumeCommand{
		RunID:              pending.RootRunID,
		CallerCapabilities: pending.Capabilities,
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
	consumeEvents(result.Events)
	if len(pending.Continuations) != 1 {
		t.Fatalf("test fixture has %d continuations, want one root only", len(pending.Continuations))
	}
	if !control.continuation.ChildRunAdmissionEnabled {
		t.Fatalf("rehydrate request = %+v, want frozen child policy restored", control.continuation)
	}
}

func TestResumeRefusesIsolatedRunAfterRuntimeRestart(t *testing.T) {
	var operations []string
	sessions := &fakeRunSessions{
		sess: session.Session{ID: "ses_1", CWD: "/work", Isolated: true},
		pending: map[string]Pending{
			"run_1": testApprovalPending("member_1", time.Now().UTC()),
		},
		operations: &operations,
	}
	// The process that owned the sandbox copy is gone (Prepare reports the execution as
	// not live), so a rehydrate would run against the real project tree.
	control := &fakeExecutionPorts{prepareErr: ErrExecutorNotLive}
	c := newUseCaseCoordinator(&fakeExecutor{}, control, sessions, &fakeEffects{})

	_, err := c.Resume(t.Context(), ResumeCommand{
		RunID: "run_1",
		CallerCapabilities: run.Capabilities{
			InterruptKinds: []interrupt.Kind{interrupt.Approval},
		},
		Responses: []ResumeResponse{{
			ItemID: "item_1", Kind: ApprovalResponseKind,
			Approval: &ApprovalResponse{Approved: true},
		}},
	})
	if !errors.Is(err, ErrRunNotFound) || !errors.Is(err, ErrExecutorStateLost) {
		t.Fatalf("Resume error = %v, want Run not found wrapping executor state lost", err)
	}
	if control.continuation.Checkpoint.RootMemberID != "" || len(control.continuation.Members) != 0 {
		t.Fatalf("isolated Run staged continuation %+v, want none", control.continuation)
	}
	if sessions.lostRunID != "run_1" || len(operations) != 1 || operations[0] != "durable.lost" {
		t.Fatalf("lost recovery = %q ops=%v, want run_1 marked lost", sessions.lostRunID, operations)
	}
	if hasActiveSession(c, "ses_1") {
		t.Fatal("failed isolated resume leaked its run admission")
	}
}

func approvalInterrupt(itemID string, occurredAt time.Time) []transcript.Interrupt {
	return []transcript.Interrupt{{
		ItemID: itemID, ItemOccurredAt: occurredAt,
		RunID: "run_1", Kind: interrupt.Approval,
		Approval: &transcript.Approval{
			Tool: transcript.ToolInvocation{Name: "shell"}, Risk: "medium",
		},
	}}
}

func testApprovalPending(memberID string, runCreatedAt time.Time) Pending {
	const interruptItemID = "item_1"

	interruptValues := approvalInterrupt(interruptItemID, runCreatedAt)
	return Pending{
		RootRunID:  "run_1",
		SessionID:  "ses_1",
		ExecutorID: "turn_1",
		Interrupts: interruptValues,
		Capabilities: run.Capabilities{
			InterruptKinds: []interrupt.Kind{interrupt.Approval},
		},
		Bindings: []InterruptBinding{{
			InterruptItemID: interruptItemID,
			MemberID:        memberID,
			RequestID:       "request_1",
		}},
		Continuations: []Continuation{{
			RunID:        "run_1",
			MemberID:     memberID,
			RunCreatedAt: runCreatedAt,
		}},
		CreatedAt: runCreatedAt.Add(time.Second),
	}
}

func runForPending(pending Pending) transcript.Run {
	root, _ := pending.RootContinuation()
	return runForContinuation(pending, root)
}

func runForContinuation(
	pending Pending,
	continuation Continuation,
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
		State:           run.Waiting,
		Metrics:         continuation.Metrics,
		Limits:          continuation.Limits,
		Capabilities:    pending.Capabilities,
		CreatedAt:       continuation.RunCreatedAt,
		MessageMark:     transcript.UnknownMessageMark,
	}
}

func TestCancelParkedRunUsesApplicationAdmission(t *testing.T) {
	var operations []string
	pending := testApprovalPending("member_1", time.Now().UTC())
	sessions := &fakeRunSessions{
		pending: map[string]Pending{
			"run_1": pending,
		},
		operations: &operations,
	}
	control := &fakeExecutionPorts{operations: &operations}
	c := NewCoordinator(Dependencies{
		Releases: control, Session: testSessionPorts(sessions),
		Runs: &fakeRunProjection{runs: map[string]transcript.Run{
			"run_1": runForPending(pending),
		}},
		Admissions: new(admission.Gate),
	})

	result, err := c.Cancel(t.Context(), CancelCommand{RunID: "run_1", Reason: "user stopped"})
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if result.Run.ID != "run_1" || result.Run.State != run.Canceled {
		t.Fatalf("Cancel result = %+v, want canceled run_1", result)
	}
	if sessions.canceledRunID != "run_1" || len(control.released) != 1 {
		t.Fatalf("durable cancel=%q execution cancels=%v", sessions.canceledRunID, control.released)
	}
	if sessions.cancelReason != "user stopped" || sessions.canceledAt.IsZero() {
		t.Fatalf("cancel reason/time = %q/%v, want user reason and terminal time", sessions.cancelReason, sessions.canceledAt)
	}
	if len(operations) != 2 || operations[0] != "durable.cancel" || operations[1] != "executor.release" {
		t.Fatalf("cancel operations = %v, want durable commit before executor cleanup", operations)
	}
	if hasActiveSession(c, "ses_1") {
		t.Fatal("parked cancel leaked the session admission claim")
	}
}

func TestCancelFinishedRunReportsFinishedInsteadOfNotFound(t *testing.T) {
	finished := runRecord(run.Completed, "", "")
	c := NewCoordinator(Dependencies{
		Releases:   &fakeExecutionPorts{},
		Session:    testSessionPorts(&fakeRunSessions{}),
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
		Releases:   &fakeExecutionPorts{},
		Session:    testSessionPorts(&fakeRunSessions{}),
		Runs:       &fakeRunProjection{},
		Admissions: new(admission.Gate),
	})

	_, err := c.Cancel(t.Context(), CancelCommand{RunID: "run_missing"})
	if !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("Cancel error = %v, want ErrRunNotFound", err)
	}
}

func TestCancelChildRunRequiresExplicitAuthority(t *testing.T) {
	child := runRecord(run.Running, "seg_child", "item_parent")
	c := NewCoordinator(Dependencies{
		Releases:   &fakeExecutionPorts{},
		Session:    testSessionPorts(&fakeRunSessions{}),
		Runs:       &fakeRunProjection{runs: map[string]transcript.Run{"run_1": child}},
		Admissions: new(admission.Gate),
	})

	_, err := c.Cancel(t.Context(), CancelCommand{RunID: "run_1"})
	if !errors.Is(err, ErrChildRunNotAllowed) {
		t.Fatalf("Cancel error = %v, want ErrChildRunNotAllowed", err)
	}
}

func projectAdmittedChildRun(
	t *testing.T,
	effects *fakeEffects,
	projection *fakeRunProjection,
) transcript.Run {
	t.Helper()
	openings := effects.openingSnapshot()
	if len(openings) != 2 || openings[1].Admit == nil {
		t.Fatalf("openings = %+v, want root and child", openings)
	}
	draft := *openings[1].Admit
	child := transcript.Run{
		ID:              draft.RunID,
		SessionID:       draft.SessionID,
		SpawnedByItemID: draft.SpawnedByItemID,
		ParentRunID:     draft.ParentRunID,
		RootRunID:       draft.RootRunID,
		State:           run.Running,
		ActiveSegmentID: draft.SegmentID,
		ModelSelection:  draft.ModelSelection,
		Limits:          draft.Limits,
		Capabilities:    draft.Capabilities,
		CreatedAt:       draft.CreatedAt,
		UpdatedAt:       draft.CreatedAt,
		MessageMark:     transcript.UnknownMessageMark,
	}
	projection.runs[child.ID] = child
	return child
}

func requireCanceledChildResult(t *testing.T, result CancelResult, child transcript.Run, reason string) {
	t.Helper()
	if result.Run.ID != child.ID || result.Run.State != run.Canceled || result.Run.Outcome == nil ||
		*result.Run.Outcome != run.OutcomeCanceled || result.Run.Detail != reason {
		t.Fatalf("child result = %+v, want exact canceled terminal", result.Run)
	}
	if result.RootRun == nil || result.RootRun.ID != child.RootRunID || result.RootRun.State != run.Running {
		t.Fatalf("root result = %+v, want still-running %s", result.RootRun, child.RootRunID)
	}
}

func requireChildCancellationProjection(
	t *testing.T,
	commits []EventCommit,
	child transcript.Run,
	reason string,
) {
	t.Helper()
	childTerminalCommits, parentCancellationItems := 0, 0
	for _, commit := range commits {
		if commit.State == StateTerminalize && commit.RunID == child.ID {
			childTerminalCommits++
		}
		for _, item := range commit.Items {
			if item.ID == child.SpawnedByItemID && item.Status == transcript.ItemIncomplete &&
				item.Error != nil && item.Error.Kind == transcript.ChildRunCanceledProblem &&
				item.Error.Scope == transcript.ToolProblem && item.Error.Detail == reason {
				parentCancellationItems++
			}
		}
	}
	if childTerminalCommits != 1 || parentCancellationItems != 1 {
		t.Fatalf(
			"child terminal commits=%d parent child_run_canceled items=%d, want 1/1",
			childTerminalCommits,
			parentCancellationItems,
		)
	}
}

func requireLiveJournalHead(t *testing.T, coordinator *Coordinator, runID string) (*journal, string) {
	t.Helper()
	entry, live := coordinator.registry.Get(runID)
	if !live || entry.owner == nil || entry.owner.hub == nil {
		t.Fatalf("continued Run %q has no event journal", runID)
	}
	subscription := entry.owner.hub.tail()
	cursor := subscription.HeadCursor
	subscription.Cancel()
	if cursor == "" {
		t.Fatalf("Run %q journal has no established cursor", runID)
	}
	return entry.owner.hub, cursor
}

func requireNaturalRootCompletion(t *testing.T, streamDone <-chan []Event, rootRunID string) {
	t.Helper()
	select {
	case events := <-streamDone:
		for _, event := range events {
			finished, ok := event.Payload.(SegmentFinished)
			if ok && event.RunID == rootRunID && finished.Run.State == run.Completed {
				return
			}
		}
		t.Fatalf("root Run %q did not continue to its natural terminal: %+v", rootRunID, events)
	case <-time.After(time.Second):
		t.Fatalf("root Run %q did not finish after child cancellation", rootRunID)
	}
}

func requireNoRunEventsAfter(t *testing.T, eventJournal *journal, cursor, runID string) {
	t.Helper()
	replayed, err := eventJournal.replay(cursor)
	if err != nil {
		t.Fatalf("replay after child cancellation: %v", err)
	}
	for _, event := range collectEvents(replayed.Events) {
		if event.RunID == runID {
			t.Fatalf("canceled child published event after Cancel returned: %+v", event)
		}
	}
}

func TestCancelRunningChildCommitsExactSubtreeBoundaryAndKeepsRootRunning(t *testing.T) {
	childRequest, childConfirmation := newChildStartFixture(time.Now().UTC())
	rootMember := ExecutorMember{MemberID: "member_root"}
	childMember := ExecutorMember{
		MemberID:    "member_child",
		ParentID:    rootMember.MemberID,
		SpawnCallID: "provider_child",
	}
	executor := &cancellableChildExecutor{
		rootMember:      rootMember,
		childMember:     childMember,
		request:         childRequest,
		confirmation:    childConfirmation,
		childOpened:     make(chan struct{}),
		cancelRequested: make(chan struct{}),
		finishRoot:      make(chan struct{}),
	}
	control := &fakeExecutionPorts{}
	control.cancelSubtree = func(ref ExecutorRef, memberID, reason string) error {
		if ref != (ExecutorRef{SessionID: "ses_1", ExecutorID: "turn_1"}) {
			return errors.New("subtree cancellation addressed the wrong execution")
		}
		if memberID != childMember.MemberID {
			return errors.New("subtree cancellation addressed the wrong executor member")
		}
		if reason != "stop delegated work" {
			return errors.New("subtree cancellation changed the product reason")
		}
		close(executor.cancelRequested)
		return nil
	}
	effects := &fakeEffects{}
	projection := &fakeRunProjection{runs: map[string]transcript.Run{
		"run_1": runForSegment(testSegment()),
	}}
	coordinator := NewCoordinator(Dependencies{
		Observations:           executor,
		Releases:               control,
		RunningSubtreeCanceler: control,
		Session:                testSessionPorts(&fakeRunSessions{}),
		Projection:             testProjectionPorts(effects),
		Runs:                   projection,
		Admissions:             new(admission.Gate),
		Now:                    func() time.Time { return time.Date(2026, 7, 30, 1, 2, 3, 0, time.UTC) },
		NewRunID:               func() string { return "run_child" },
		NewSegmentID:           func() string { return "seg_child" },
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

	childRun := projectAdmittedChildRun(t, effects, projection)

	result, err := coordinator.Cancel(t.Context(), CancelCommand{
		RunID:         childRun.ID,
		Reason:        "stop delegated work",
		AllowChildRun: true,
	})
	if err != nil {
		t.Fatalf("Cancel child: %v", err)
	}
	requireCanceledChildResult(t, result, childRun, "stop delegated work")
	requireChildCancellationProjection(t, effects.commitSnapshot(), childRun, "stop delegated work")
	if _, live := coordinator.registry.Get("run_1"); !live {
		t.Fatal("child cancellation stopped the root segment")
	}
	eventJournal, cancellationCursor := requireLiveJournalHead(t, coordinator, "run_1")

	close(executor.finishRoot)
	requireNaturalRootCompletion(t, streamDone, "run_1")
	requireNoRunEventsAfter(t, eventJournal, cancellationCursor, childRun.ID)
}

func TestCancelParkedRunReportsExecutorReleaseFailureAfterDurableCommit(t *testing.T) {
	cleanupErr := errors.New("executor release failed")
	pending := testApprovalPending("member_1", time.Now().UTC())
	sessions := &fakeRunSessions{pending: map[string]Pending{
		"run_1": pending,
	}}
	control := &fakeExecutionPorts{releaseErr: cleanupErr}
	c := NewCoordinator(Dependencies{
		Releases: control, Session: testSessionPorts(sessions),
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
		t.Fatal("executor release failure prevented the durable cancel commit")
	}
}

func TestCancelLiveRunReportsExecutorReleaseFailureAndStillTerminalizes(t *testing.T) {
	cleanupErr := errors.New("executor release failed")
	executor := &fakeExecutor{block: true, releaseErr: cleanupErr}
	effects := &fakeEffects{}
	control := &fakeExecutionPorts{releaseErr: cleanupErr}
	c := NewCoordinator(Dependencies{
		Observations: executor, Releases: control, RootCancellation: executor, Session: testSessionPorts(&fakeRunSessions{}), Projection: testProjectionPorts(effects),
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
	consumePulledEvents(next) // drain the terminal events
	if !effects.terminalized("ses_1", "run_1") {
		t.Fatal("executor release failure prevented live Run terminalization")
	}
}

func TestCancelLiveRunJoinsTerminalMaintenance(t *testing.T) {
	finishStarted := make(chan struct{}, 1)
	releaseFinish := make(chan struct{})
	executor := &fakeExecutor{block: true}
	effects := &fakeEffects{finishStarted: finishStarted, finishRelease: releaseFinish}
	control := &fakeExecutionPorts{startRef: ExecutorRef{SessionID: "ses_1", ExecutorID: "turn_1"}}
	sessions := &fakeRunSessions{sess: session.Session{ID: "ses_1", CWD: "/work"}}
	c := newUseCaseCoordinator(executor, control, sessions, effects)
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
		outcome.result.Run.State != run.Canceled ||
		outcome.result.Run.Outcome == nil ||
		*outcome.result.Run.Outcome != run.OutcomeCanceled ||
		outcome.result.Run.Detail != "stop" {
		t.Fatalf("Cancel result = %+v, want exact canceled terminal snapshot", outcome.result)
	}
	if hasActiveSession(c, "ses_1") {
		t.Fatal("Cancel returned before releasing session admission")
	}
	consumeEvents(result.Events)
}

func TestCancelLosesToACommittedNaturalTerminal(t *testing.T) {
	terminalStarted := make(chan struct{}, 1)
	releaseTerminal := make(chan struct{})
	executor := &fakeExecutor{events: []ExecutorPayload{SegmentEnded{
		Reason: run.OutcomeCompleted,
	}}}
	effects := &fakeEffects{terminalStarted: terminalStarted, terminalRelease: releaseTerminal}
	control := &fakeExecutionPorts{}
	c := NewCoordinator(Dependencies{
		Observations: executor, Releases: control, RootCancellation: executor, Session: testSessionPorts(&fakeRunSessions{}), Projection: testProjectionPorts(effects),
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
	for entry.owner.CancelReason() != "too late" {
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
		ToolCallStarted{
			CallID: "call_1", ToolName: "shell", Arguments: `{"command":"pwd","description":"Print the working directory"}`,
			SafetyClass: "write",
		},
		TreeInterrupted{Checkpoint: testExecutorCheckpoint(), Interruptions: []MemberInterruption{{
			MemberID: "member_root", RequestID: "request_1",
			Interrupt: Interrupt{
				Kind: interrupt.Approval,
				Approval: &ApprovalPrompt{
					CallID: "call_1", ToolName: "shell", Arguments: `{"command":"pwd","description":"Print the working directory"}`,
					SafetyClass: "write", Risk: "medium",
				},
			},
		}}},
	}}
	effects := &fakeEffects{
		suspendStarted: suspendStarted, suspendCanceled: suspendCanceled,
		suspendRelease: releaseSuspend,
	}
	var operations []string
	control := &fakeExecutionPorts{operations: &operations}
	sessions := &fakeRunSessions{operations: &operations}
	spec := testSegment()
	spec.Capabilities = run.Capabilities{
		InterruptKinds: []interrupt.Kind{interrupt.Approval},
	}
	c := NewCoordinator(Dependencies{
		Observations: executor, Releases: control, RootCancellation: executor, Session: testSessionPorts(sessions), Projection: testProjectionPorts(effects),
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

	if executor.releases() != 0 {
		t.Fatalf("pump executor releases = %d, want parked owner to remain intact until durable cancel", executor.releases())
	}
	if len(operations) != 2 || operations[0] != "durable.cancel" || operations[1] != "executor.release" {
		t.Fatalf("cancel operations = %v, want durable cancel before parked executor release", operations)
	}
}

func TestCancelTreatsAlreadyReleasedExecutorAsIdempotentSuccess(t *testing.T) {
	pending := testApprovalPending("member_1", time.Now().UTC())
	sessions := &fakeRunSessions{pending: map[string]Pending{
		"run_1": pending,
	}}
	control := &fakeExecutionPorts{releaseErr: ErrExecutorNotLive}
	c := NewCoordinator(Dependencies{
		Releases: control, Session: testSessionPorts(sessions),
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
	control := &fakeExecutionPorts{}
	c, _ := liveCoordinator(t, runRecord(run.Running, testSegmentID, ""))
	c.steering = control

	if err := c.Steer(context.Background(), SteerCommand{
		RunID:             testRunID,
		ExpectedSegmentID: testSegmentID,
		Input: []transcript.ContentBlock{
			{Kind: transcript.TextContent, Text: "wait"},
			{Kind: transcript.ImageContent, MediaType: "image/png", Bytes: []byte("image")},
		},
	}); err != nil {
		t.Fatalf("Steer: %v", err)
	}
	if len(control.steered) != 1 || control.steered[0] != (ExecutorRef{SessionID: "ses_1", ExecutorID: "turn_1"}) {
		t.Fatalf("steered refs = %+v", control.steered)
	}
	if len(control.steerInput) != 2 ||
		control.steerInput[0].Text != "wait" ||
		control.steerInput[1].Kind != transcript.ImageContent {
		t.Fatalf("steer input = %+v", control.steerInput)
	}
}

func TestStartRejectsInvalidInputBeforeSessionCreation(t *testing.T) {
	sessions := &fakeRunSessions{}
	c := newUseCaseCoordinator(&fakeExecutor{}, &fakeExecutionPorts{}, sessions, &fakeEffects{})

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
		state  run.State
		status run.Status
	}{
		{"a running run", run.Running, run.StatusRunning},
		{"a run waiting on a person", run.Waiting, run.StatusWaiting},
	} {
		t.Run(tt.name, func(t *testing.T) {
			effects := &fakeEffects{}
			sessions := &fakeRunSessions{
				sess:   session.Session{ID: "ses_1", CWD: "/work"},
				active: &transcript.Run{ID: "run_active", SessionID: "ses_1", State: tt.state},
			}
			c := newUseCaseCoordinator(&fakeExecutor{}, &fakeExecutionPorts{}, sessions, effects)

			_, err := c.Start(context.Background(), StartCommand{
				SessionID:      "ses_1",
				ModelSelection: mustUseCaseSelection("provider", "model"),
				Input:          []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "hi"}},
			})
			conflict, ok := errors.AsType[*ActiveRunConflictError](err)
			if !ok {
				t.Fatalf("Start = %v, want an ActiveRunConflictError", err)
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
