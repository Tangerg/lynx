package runs

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

type fakeRunSessions struct {
	sess          session.Session
	createdTitle  string
	model         string
	pending       map[string]interrupts.Pending
	canceledRunID string
	treeOK        bool
	treeReleases  int
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

func (f *fakeRunSessions) ApplyRunCancel(_ context.Context, _ string, runID string) error {
	f.canceledRunID = runID
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
	startTurn    Turn
	prepared     Turn
	prepareErr   error
	rehydrated   Turn
	resumeCheck  func()
	resumed      bool
	canceled     []TurnRef
	steered      []TurnRef
	steerMessage string
}

func (f *fakeTurnControl) ValidateStart(req StartTurn) error {
	f.validated = req
	return nil
}

func (f *fakeTurnControl) Start(_ context.Context, req StartTurn) (Turn, error) {
	f.started = req
	return f.startTurn, nil
}

func (f *fakeTurnControl) Prepare(context.Context, TurnRef) (Turn, error) {
	return f.prepared, f.prepareErr
}

func (f *fakeTurnControl) Resume(context.Context, Turn, interrupts.Resolution, []string) error {
	if f.resumeCheck != nil {
		f.resumeCheck()
	}
	f.resumed = true
	return nil
}

func (f *fakeTurnControl) Rehydrate(context.Context, RehydrateTurn) (Turn, error) {
	return f.rehydrated, nil
}

func (f *fakeTurnControl) Cancel(_ context.Context, ref TurnRef) error {
	f.canceled = append(f.canceled, ref)
	return nil
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
	turns := &fakeTurnControl{startTurn: Turn{SessionID: "ses_1", TurnID: "turn_1", Handle: "opaque"}}
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

func TestResumeCommitsOpeningBeforeActivation(t *testing.T) {
	createdAt := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	effects := &fakeEffects{}
	sessions := &fakeRunSessions{
		sess:   session.Session{ID: "ses_1", Cwd: "/work"},
		treeOK: true,
		pending: map[string]interrupts.Pending{"run_1": {
			RunID: "run_1", SessionID: "ses_1", TurnID: "turn_1", RunCreatedAt: createdAt,
		}},
	}
	turns := &fakeTurnControl{prepared: Turn{SessionID: "ses_1", TurnID: "turn_1", Handle: "opaque"}}
	activatedAfterOpening := false
	turns.resumeCheck = func() { activatedAfterOpening = effects.opening().Resume != nil }
	c := newUseCaseCoordinator(&fakeExecutor{}, turns, sessions, effects)

	result, err := c.Resume(context.Background(), ResumeCommand{
		RunID:      "run_1",
		Resolution: interrupts.Resolution{Approved: true},
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

func TestCancelParkedRunUsesApplicationAdmission(t *testing.T) {
	sessions := &fakeRunSessions{pending: map[string]interrupts.Pending{"run_1": {
		RunID: "run_1", SessionID: "ses_1", TurnID: "turn_1",
	}}}
	turns := &fakeTurnControl{}
	c := NewCoordinator(Dependencies{Turns: turns, Sessions: sessions})

	if err := c.Cancel(context.Background(), CancelCommand{RunID: "run_1"}); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if sessions.canceledRunID != "run_1" || len(turns.canceled) != 1 {
		t.Fatalf("durable cancel=%q turn cancels=%v", sessions.canceledRunID, turns.canceled)
	}
	if c.ActiveSession("ses_1") {
		t.Fatal("parked cancel leaked the session admission claim")
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
