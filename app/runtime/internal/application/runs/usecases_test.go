package runs

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

type fakeRunSessions struct {
	sess          session.Session
	createdTitle  string
	model         string
	pending       map[string]interrupts.Pending
	canceledRunID string
	cancelReason  string
	canceledAt    time.Time
	lostRunID     string
	lostAt        time.Time
	treeOK        bool
	treeReleases  int
	operations    *[]string
}

func (f *fakeRunSessions) Get(context.Context, string) (session.Session, error) {
	return f.sess, nil
}

func (f *fakeRunSessions) Create(_ context.Context, title, cwd string) (session.Session, error) {
	f.createdTitle = title
	f.sess = session.Session{ID: "ses_created", Cwd: cwd}
	return f.sess, nil
}

func (f *fakeRunSessions) SetModel(_ context.Context, _ string, model string) error {
	f.model = model
	return nil
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

func (f *fakeRunSessions) ApplyRunCancel(_ context.Context, _ string, runID, reason string, finishedAt time.Time) error {
	if f.operations != nil {
		*f.operations = append(*f.operations, "durable.cancel")
	}
	f.canceledRunID = runID
	f.cancelReason = reason
	f.canceledAt = finishedAt
	delete(f.pending, runID)
	return nil
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

func (f *fakeRunSessions) AcquireWorkingTreeRun(string) (func(), bool) {
	if !f.treeOK {
		return nil, false
	}
	var once sync.Once
	return func() { once.Do(func() { f.treeReleases++ }) }, true
}

type fakeTurnControl struct {
	validated    StartTurn
	started      StartTurn
	startTurn    TurnRef
	prepared     TurnRef
	prepareErr   error
	rehydrated   TurnRef
	rehydrateReq RehydrateTurn
	rehydrateErr error
	resumeCheck  func()
	resumed      bool
	canceled     []TurnRef
	steered      []TurnRef
	steerMessage string
	operations   *[]string
	cancelErr    error
}

func (f *fakeTurnControl) ValidateStart(req StartTurn) error {
	f.validated = req
	return nil
}

func (f *fakeTurnControl) Start(_ context.Context, req StartTurn) (TurnRef, error) {
	f.started = req
	return f.startTurn, nil
}

func (f *fakeTurnControl) Prepare(context.Context, TurnRef) (TurnRef, error) {
	return f.prepared, f.prepareErr
}

func (f *fakeTurnControl) Resume(context.Context, TurnRef, interrupts.Resolution, []string) error {
	if f.resumeCheck != nil {
		f.resumeCheck()
	}
	f.resumed = true
	return nil
}

func (f *fakeTurnControl) Rehydrate(_ context.Context, request RehydrateTurn) (TurnRef, error) {
	f.rehydrateReq = request
	return f.rehydrated, f.rehydrateErr
}

func (f *fakeTurnControl) Cancel(_ context.Context, ref TurnRef) error {
	if f.operations != nil {
		*f.operations = append(*f.operations, "turn.cancel")
	}
	f.canceled = append(f.canceled, ref)
	return f.cancelErr
}

func (f *fakeTurnControl) Steer(_ context.Context, ref TurnRef, message string) error {
	f.steered = append(f.steered, ref)
	f.steerMessage = message
	return nil
}

func newUseCaseCoordinator(exec SegmentExecutor, turns TurnControl, sessions SessionLifecycle, effects Effects) *Coordinator {
	return NewCoordinator(Dependencies{
		Segments:     exec,
		Turns:        turns,
		Sessions:     sessions,
		Effects:      effects,
		Now:          func() time.Time { return time.Date(2026, 7, 13, 1, 2, 3, 0, time.UTC) },
		NewRunID:     func() string { return "run_new" },
		NewSegmentID: func() string { return "seg_new" },
	})
}

func TestStartOwnsCompleteAdmissionSequence(t *testing.T) {
	exec := &fakeExecutor{}
	effects := &fakeEffects{}
	sessions := &fakeRunSessions{sess: session.Session{ID: "ses_1", Cwd: "/work"}, treeOK: true}
	turns := &fakeTurnControl{startTurn: TurnRef{SessionID: "ses_1", TurnID: "turn_1"}}
	c := newUseCaseCoordinator(exec, turns, sessions, effects)

	result, err := c.Start(context.Background(), StartCommand{
		SessionID:       "ses_1",
		Message:         "hello",
		Provider:        "provider",
		Model:           "model",
		OpeningUserText: "hello",
		Input:           []ContentBlock{{Kind: TextContent, Text: "hello"}},
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
	if sessions.model != "model" {
		t.Fatalf("recorded model = %q", sessions.model)
	}
	if sessions.treeReleases != 1 {
		t.Fatalf("working-tree releases = %d, want 1", sessions.treeReleases)
	}
	if opening := effects.opening(); opening.Admit == nil || opening.Admit.RunID != "run_new" {
		t.Fatalf("opening = %+v, want fresh run admission", opening)
	}
}

func TestStartRejectsForeignTurnIdentityAndCleansItUp(t *testing.T) {
	exec := &fakeExecutor{}
	effects := &fakeEffects{}
	sessions := &fakeRunSessions{sess: session.Session{ID: "ses_1", Cwd: "/work"}, treeOK: true}
	turns := &fakeTurnControl{startTurn: TurnRef{SessionID: "ses_foreign", TurnID: "turn_1"}}
	c := newUseCaseCoordinator(exec, turns, sessions, effects)

	_, err := c.Start(context.Background(), StartCommand{SessionID: "ses_1", Message: "hello"})
	if !errors.Is(err, ErrInvalidTurnRef) {
		t.Fatalf("Start error = %v, want ErrInvalidTurnRef", err)
	}
	if len(turns.canceled) != 1 || turns.canceled[0] != turns.startTurn {
		t.Fatalf("canceled turns = %+v, want invalid started turn", turns.canceled)
	}
	if len(effects.openings) != 0 || c.Contains("run_new") {
		t.Fatal("invalid turn identity reached run admission")
	}
}

func TestResumeCommitsOpeningBeforeActivation(t *testing.T) {
	createdAt := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	effects := &fakeEffects{}
	sessions := &fakeRunSessions{
		sess:   session.Session{ID: "ses_1", Cwd: "/work"},
		treeOK: true,
		pending: map[string]interrupts.Pending{"run_1": {
			RunID: "run_1", SessionID: "ses_1", TurnID: "turn_1", RunCreatedAt: createdAt,
			Interrupts: approvalInterrupt("item_1"),
		}},
	}
	turns := &fakeTurnControl{prepared: TurnRef{SessionID: "ses_1", TurnID: "turn_1"}}
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

func TestResumeRecoversLostProcessSnapshotBeforeReturning(t *testing.T) {
	var operations []string
	sessions := &fakeRunSessions{
		sess:   session.Session{ID: "ses_1", Cwd: "/work"},
		treeOK: true,
		pending: map[string]interrupts.Pending{"run_1": {
			RunID: "run_1", SessionID: "ses_1", TurnID: "turn_1", ProcessID: "proc_1",
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
	if sessions.treeReleases != 1 || c.ActiveSession("ses_1") {
		t.Fatalf("tree releases = %d active session = %v", sessions.treeReleases, c.ActiveSession("ses_1"))
	}

	_, err = c.Resume(t.Context(), ResumeCommand{RunID: "run_1"})
	if !errors.Is(err, ErrInterruptNotOpen) {
		t.Fatalf("second Resume error = %v, want ErrInterruptNotOpen", err)
	}
	if len(operations) != 1 {
		t.Fatalf("second Resume repeated recovery: %v", operations)
	}
}

func approvalInterrupt(itemID string) []transcript.Interrupt {
	return []transcript.Interrupt{{
		ItemID: itemID,
		Kind:   transcript.ApprovalInterrupt,
		Approval: &transcript.Approval{
			Tool: transcript.ToolInvocation{Name: "shell"},
		},
	}}
}

func TestCancelParkedRunUsesApplicationAdmission(t *testing.T) {
	var operations []string
	sessions := &fakeRunSessions{pending: map[string]interrupts.Pending{"run_1": {
		RunID: "run_1", SessionID: "ses_1", TurnID: "turn_1",
	}}, operations: &operations}
	turns := &fakeTurnControl{operations: &operations}
	c := NewCoordinator(Dependencies{Turns: turns, Sessions: sessions})

	if err := c.Cancel(t.Context(), CancelCommand{RunID: "run_1", Reason: "user stopped"}); err != nil {
		t.Fatalf("Cancel: %v", err)
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
	if c.ActiveSession("ses_1") {
		t.Fatal("parked cancel leaked the session admission claim")
	}
}

func TestCancelParkedRunReportsTurnCleanupFailureAfterDurableCommit(t *testing.T) {
	cleanupErr := errors.New("turn cleanup failed")
	sessions := &fakeRunSessions{pending: map[string]interrupts.Pending{"run_1": {
		RunID: "run_1", SessionID: "ses_1", TurnID: "turn_1",
	}}}
	turns := &fakeTurnControl{cancelErr: cleanupErr}
	c := NewCoordinator(Dependencies{Turns: turns, Sessions: sessions})

	err := c.Cancel(t.Context(), CancelCommand{RunID: "run_1", Reason: "stop"})
	if !errors.Is(err, cleanupErr) {
		t.Fatalf("Cancel error = %v, want cleanup failure", err)
	}
	if sessions.canceledRunID != "run_1" {
		t.Fatal("turn cleanup failure prevented the durable cancel commit")
	}
}

func TestCancelLiveRunReportsTurnCleanupFailureAndStillTerminalizes(t *testing.T) {
	cleanupErr := errors.New("turn cleanup failed")
	executor := &fakeExecutor{block: true}
	effects := &fakeEffects{}
	turns := &fakeTurnControl{cancelErr: cleanupErr}
	c := NewCoordinator(Dependencies{Segments: executor, Turns: turns, Sessions: &fakeRunSessions{}, Effects: effects})
	stream, err := c.openSegment(t.Context(), testSegment())
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	<-stream

	err = c.Cancel(t.Context(), CancelCommand{RunID: "run_1", Reason: "stop"})
	if !errors.Is(err, cleanupErr) {
		t.Fatalf("Cancel error = %v, want cleanup failure", err)
	}
	collectEvents(stream)
	if !effects.terminalized("ses_1", "run_1") {
		t.Fatal("turn cleanup failure prevented live run terminalization")
	}
}

func TestCancelTreatsAlreadyGoneTurnAsIdempotentSuccess(t *testing.T) {
	sessions := &fakeRunSessions{pending: map[string]interrupts.Pending{"run_1": {
		RunID: "run_1", SessionID: "ses_1", TurnID: "turn_1",
	}}}
	turns := &fakeTurnControl{cancelErr: ErrTurnNotLive}
	c := NewCoordinator(Dependencies{Turns: turns, Sessions: sessions})

	if err := c.Cancel(t.Context(), CancelCommand{RunID: "run_1"}); err != nil {
		t.Fatalf("Cancel error = %v, want idempotent success", err)
	}
}

func TestSteerHidesExecutorHandle(t *testing.T) {
	turns := &fakeTurnControl{}
	c := NewCoordinator(Dependencies{Turns: turns})
	c.registry.Open(Record{ID: "run_1", SessionID: "ses_1", TurnID: "turn_1"}, nil)

	if err := c.Steer(context.Background(), SteerCommand{RunID: "run_1", Message: "wait"}); err != nil {
		t.Fatalf("Steer: %v", err)
	}
	if len(turns.steered) != 1 || turns.steered[0] != (TurnRef{SessionID: "ses_1", TurnID: "turn_1"}) {
		t.Fatalf("steered refs = %+v", turns.steered)
	}
	if turns.steerMessage != "wait" {
		t.Fatalf("steer message = %q", turns.steerMessage)
	}
}

func TestStartRejectsInvalidInputBeforeSessionCreation(t *testing.T) {
	sessions := &fakeRunSessions{treeOK: true}
	c := newUseCaseCoordinator(&fakeExecutor{}, &fakeTurnControl{}, sessions, &fakeEffects{})

	_, err := c.Start(context.Background(), StartCommand{})
	if !errors.Is(err, ErrInputRequired) {
		t.Fatalf("err = %v, want ErrInputRequired", err)
	}
	if sessions.sess.ID != "" {
		t.Fatalf("invalid input created session %+v", sessions.sess)
	}
}
