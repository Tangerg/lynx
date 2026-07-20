package runs

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
)

// These fakes exercise the application-owned reducer and journal. Delivery
// protocol values deliberately do not appear here.
type fakeExecutor struct {
	events        []EngineEvent
	block         bool
	mu            sync.Mutex
	canceled      int
	startErr      error
	cancelErr     error
	cancelStarted chan struct{}
	releaseCancel chan struct{}
}

func (f *fakeExecutor) TurnEvents(ctx context.Context, _ TurnRef) (iter.Seq[EngineEvent], error) {
	if f.startErr != nil {
		return nil, f.startErr
	}
	return func(yield func(EngineEvent) bool) {
		if f.block {
			<-ctx.Done()
			return
		}
		for _, event := range f.events {
			if ctx.Err() != nil || !yield(event) {
				return
			}
		}
	}, nil
}

func (f *fakeExecutor) CancelTurn(context.Context, TurnRef) error {
	if f.cancelStarted != nil {
		close(f.cancelStarted)
	}
	if f.releaseCancel != nil {
		<-f.releaseCancel
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.canceled++
	return f.cancelErr
}

func (f *fakeExecutor) cancels() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.canceled
}

type fakeEffects struct {
	mu             sync.Mutex
	commits        []EventCommit
	openings       []OpeningCommit
	finishes       []Finish
	nudges         int
	openingErr     error
	commitErr      error
	rejectCanceled bool
	finishStarted  chan<- struct{}
	finishRelease  <-chan struct{}
}

func (e *fakeEffects) CommitOpening(_ context.Context, opening OpeningCommit) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.openingErr != nil {
		return e.openingErr
	}
	e.openings = append(e.openings, opening)
	e.commits = append(e.commits, opening.Events...)
	return nil
}

func (e *fakeEffects) CommitEvent(ctx context.Context, commit EventCommit) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.rejectCanceled && ctx.Err() != nil {
		return ctx.Err()
	}
	if e.commitErr != nil {
		return e.commitErr
	}
	e.commits = append(e.commits, commit)
	return nil
}

func (e *fakeEffects) Nudge(string, []string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.nudges++
}

func (e *fakeEffects) Finish(_ context.Context, finish Finish) error {
	e.mu.Lock()
	e.finishes = append(e.finishes, finish)
	e.mu.Unlock()
	if e.finishStarted != nil {
		e.finishStarted <- struct{}{}
	}
	if e.finishRelease != nil {
		<-e.finishRelease
	}
	return nil
}

func (e *fakeEffects) opening() OpeningCommit {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.openings) == 0 {
		return OpeningCommit{}
	}
	return e.openings[len(e.openings)-1]
}

func (e *fakeEffects) terminalized(sessionID, runID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, commit := range e.commits {
		if commit.State == StateTerminalize && commit.SessionID == sessionID && commit.RunID == runID {
			return true
		}
	}
	return false
}

func (e *fakeEffects) finishCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.finishes)
}

func testCoordinator(executor SegmentExecutor, effects Effects) *Coordinator {
	return NewCoordinator(Dependencies{
		Segments: executor,
		Effects:  effects,
		Now: func() time.Time {
			return time.Date(2026, 7, 13, 1, 2, 3, 0, time.UTC)
		},
	})
}

func testSegment() segmentSpec {
	return segmentSpec{
		RunID: "run_1", SegmentID: "seg_1", SessionID: "ses_1",
		TurnID: "turn_1", Provider: "openai", Model: "model",
		CreatedAt: time.Date(2026, 7, 13, 1, 2, 3, 0, time.UTC),
	}
}

func collectEvents(events <-chan Event) []Event {
	var out []Event
	for event := range events {
		out = append(out, event)
	}
	return out
}

func TestCoordinatorRejectsUncommittedOpening(t *testing.T) {
	executor := &fakeExecutor{}
	effects := &fakeEffects{openingErr: execution.ErrSessionBusy}
	coordinator := testCoordinator(executor, effects)

	events, err := coordinator.openSegment(context.Background(), testSegment())
	if !errors.Is(err, execution.ErrSessionBusy) {
		t.Fatalf("openSegment error = %v, want ErrSessionBusy", err)
	}
	if events != nil || coordinator.Contains("run_1") {
		t.Fatal("an uncommitted opening became visible")
	}
	if executor.cancels() != 1 {
		t.Fatalf("CancelTurn calls = %d, want 1", executor.cancels())
	}
	if effects.finishCount() != 0 {
		t.Fatalf("Finish calls = %d, want none without a committed terminal", effects.finishCount())
	}
}

func TestCoordinatorPreservesUnadmittedTurnCleanupFailure(t *testing.T) {
	cleanupErr := errors.New("executor cleanup failed")
	executor := &fakeExecutor{cancelErr: cleanupErr}
	openingErr := errors.New("opening commit failed")
	coordinator := testCoordinator(executor, &fakeEffects{openingErr: openingErr})

	_, err := coordinator.openSegment(t.Context(), testSegment())
	if !errors.Is(err, openingErr) || !errors.Is(err, cleanupErr) {
		t.Fatalf("openSegment error = %v, want opening and cleanup failures", err)
	}
	if executor.cancels() != 1 {
		t.Fatalf("CancelTurn calls = %d, want 1", executor.cancels())
	}
}

func TestCoordinatorCommitsCanonicalOpeningAndTerminal(t *testing.T) {
	executor := &fakeExecutor{events: []EngineEvent{
		MessageDelta{Text: "hello"},
		TurnEnd{Reason: execution.OutcomeCompleted},
	}}
	effects := &fakeEffects{}
	coordinator := testCoordinator(executor, effects)
	spec := testSegment()
	spec.Input = []ContentBlock{{Kind: TextContent, Text: "question"}}

	stream, err := coordinator.openSegment(context.Background(), spec)
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	events := collectEvents(stream)
	if len(events) < 2 {
		t.Fatalf("events = %d, want canonical opening and terminal", len(events))
	}
	started, ok := events[0].Payload.(SegmentStarted)
	if !ok || started.Run.ID != "run_1" || started.Run.SessionID != "ses_1" {
		t.Fatalf("first payload = %#v", events[0].Payload)
	}
	finished, ok := events[len(events)-1].Payload.(SegmentFinished)
	if !ok || finished.Run.Outcome == nil || *finished.Run.Outcome != execution.OutcomeCompleted {
		t.Fatalf("last payload = %#v", events[len(events)-1].Payload)
	}
	if opening := effects.opening(); opening.Admit == nil || opening.Resume != nil || len(opening.Events) != 2 {
		t.Fatalf("opening = %+v, want admit + run/user-item commits", opening)
	}
	if !effects.terminalized("ses_1", "run_1") {
		t.Fatal("terminal run and exact run-state transition were not committed")
	}
	for index := 1; index < len(events); index++ {
		if events[index-1].Seq >= events[index].Seq {
			t.Fatalf("event cursors are not monotonic: %q then %q", events[index-1].Seq, events[index].Seq)
		}
	}
}

func TestCoordinatorHoldsSessionAdmissionThroughTerminalMaintenance(t *testing.T) {
	executor := &fakeExecutor{events: []EngineEvent{TurnEnd{Reason: execution.OutcomeCompleted}}}
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	effects := &fakeEffects{finishStarted: started, finishRelease: release}
	coordinator := testCoordinator(executor, effects)

	stream, err := coordinator.openSegment(t.Context(), testSegment())
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	_ = collectEvents(stream)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("terminal maintenance did not start")
	}
	if coordinator.registry.Contains("run_1") {
		t.Fatal("terminal run remained in the live registry during maintenance")
	}
	if !coordinator.registry.ActiveSession("ses_1") {
		t.Fatal("session admission was released before terminal maintenance completed")
	}
	if _, ok := coordinator.registry.AcquireSession("ses_1"); ok {
		t.Fatal("new run admission crossed the terminal-maintenance fence")
	}

	close(release)
	coordinator.Close()
	if coordinator.registry.ActiveSession("ses_1") {
		t.Fatal("terminal-maintenance claim was not released")
	}
}

func TestCoordinatorCommitsProcessCreationFailureInCanonicalOrder(t *testing.T) {
	executor := &fakeExecutor{events: []EngineEvent{
		TurnStart{Model: "model"},
		ErrorEvent{
			Message: "engine: start chat: duplicate process extension",
			Code:    ErrorCodeEngine, Problem: Problem{Kind: InternalProblem, Scope: RunProblem, Detail: "the run failed due to an internal error"},
		},
		TurnEnd{Reason: execution.OutcomeError},
	}}
	effects := &fakeEffects{}
	coordinator := testCoordinator(executor, effects)

	stream, err := coordinator.openSegment(context.Background(), testSegment())
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	events := collectEvents(stream)
	if len(events) != 2 {
		t.Fatalf("journal events = %d, want opening and terminal", len(events))
	}
	if _, ok := events[0].Payload.(SegmentStarted); !ok {
		t.Fatalf("first payload = %#v, want SegmentStarted", events[0].Payload)
	}
	finished, ok := events[1].Payload.(SegmentFinished)
	if !ok {
		t.Fatalf("second payload = %#v, want SegmentFinished", events[1].Payload)
	}
	if finished.Run.Outcome == nil || *finished.Run.Outcome != execution.OutcomeError {
		t.Fatalf("outcome = %v, want error", finished.Run.Outcome)
	}
	if finished.Run.Result == nil || finished.Run.Result.Error == nil || finished.Run.Result.Error.Kind != InternalProblem {
		t.Fatalf("run result = %+v, want canonical internal problem", finished.Run.Result)
	}
	if events[0].Seq >= events[1].Seq {
		t.Fatalf("event order = %q then %q, want monotonic", events[0].Seq, events[1].Seq)
	}
	if !effects.terminalized("ses_1", "run_1") {
		t.Fatal("process creation failure did not atomically terminalize the run")
	}
}

func TestCoordinatorResumeCommitsBeforeActivation(t *testing.T) {
	executor := &fakeExecutor{}
	effects := &fakeEffects{}
	coordinator := testCoordinator(executor, effects)
	spec := testSegment()
	spec.SegmentID = "seg_2"
	spec.Pending = &interrupts.Pending{}
	activatedAfterOpening := false
	spec.Activate = func(context.Context) error {
		activatedAfterOpening = effects.opening().Resume != nil
		return nil
	}

	stream, err := coordinator.openSegment(context.Background(), spec)
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	collectEvents(stream)
	if !activatedAfterOpening {
		t.Fatal("continuation activated before its opening commit")
	}
	opening := effects.opening()
	if opening.Resume == nil || opening.Resume.RunID != "run_1" || opening.Admit != nil {
		t.Fatalf("opening = %+v, want resume run_1", opening)
	}
}

func TestCoordinatorActivationFailureBecomesErrorTerminal(t *testing.T) {
	executor := &fakeExecutor{block: true}
	effects := &fakeEffects{}
	coordinator := testCoordinator(executor, effects)
	spec := testSegment()
	spec.Activate = func(context.Context) error { return errors.New("resume failed") }

	stream, err := coordinator.openSegment(context.Background(), spec)
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	events := collectEvents(stream)
	finished, ok := events[len(events)-1].Payload.(SegmentFinished)
	if !ok || finished.Run.Outcome == nil || *finished.Run.Outcome != execution.OutcomeError {
		t.Fatalf("last payload = %#v, want error terminal", events[len(events)-1].Payload)
	}
	if finished.Run.Result == nil || finished.Run.Result.Error == nil {
		t.Fatalf("error terminal has no canonical problem: %+v", finished.Run)
	}
}

func TestCoordinatorMalformedInterruptAbortsExecutorAndTerminalizes(t *testing.T) {
	executor := &fakeExecutor{events: []EngineEvent{TurnInterrupted{Interrupts: []Interrupt{{Kind: InterruptKind("unknown")}}}}}
	effects := &fakeEffects{}
	coordinator := testCoordinator(executor, effects)

	stream, err := coordinator.openSegment(t.Context(), testSegment())
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	events := collectEvents(stream)
	finished, ok := events[len(events)-1].Payload.(SegmentFinished)
	if !ok || finished.Run.Outcome == nil || *finished.Run.Outcome != execution.OutcomeError {
		t.Fatalf("last payload = %#v, want error terminal", events[len(events)-1].Payload)
	}
	if executor.cancels() != 1 {
		t.Fatalf("CancelTurn calls = %d, want 1", executor.cancels())
	}
	if !effects.terminalized("ses_1", "run_1") {
		t.Fatal("malformed interrupt did not terminalize the run")
	}
}

func TestCoordinatorProtocolViolationAbortsExecutorAndTerminalizes(t *testing.T) {
	tests := []struct {
		name  string
		event EngineEvent
	}{
		{name: "unknown event", event: unsupportedEngineEvent{}},
		{name: "invalid terminal outcome", event: TurnEnd{Reason: execution.Outcome(255)}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			executor := &fakeExecutor{events: []EngineEvent{test.event}}
			effects := &fakeEffects{}
			coordinator := testCoordinator(executor, effects)

			stream, err := coordinator.openSegment(t.Context(), testSegment())
			if err != nil {
				t.Fatalf("openSegment: %v", err)
			}
			events := collectEvents(stream)
			if len(events) != 2 {
				t.Fatalf("journal events = %d, want opening and synthesized terminal", len(events))
			}
			finished, ok := events[1].Payload.(SegmentFinished)
			if !ok || finished.Run.Outcome == nil || *finished.Run.Outcome != execution.OutcomeError {
				t.Fatalf("last payload = %#v, want error terminal", events[1].Payload)
			}
			if finished.Run.Result == nil || finished.Run.Result.Error == nil || finished.Run.Result.Error.Kind != InternalProblem {
				t.Fatalf("run result = %+v, want canonical internal problem", finished.Run.Result)
			}
			if executor.cancels() != 1 {
				t.Fatalf("CancelTurn calls = %d, want 1", executor.cancels())
			}
			if !effects.terminalized("ses_1", "run_1") {
				t.Fatal("executor protocol violation did not terminalize the run")
			}
		})
	}
}

func TestCoordinatorCommitsSyntheticTerminalBeforeCancelTurn(t *testing.T) {
	executor := &fakeExecutor{
		events:        []EngineEvent{TurnInterrupted{Interrupts: []Interrupt{{Kind: InterruptKind("unknown")}}}},
		cancelStarted: make(chan struct{}),
		releaseCancel: make(chan struct{}),
	}
	effects := &fakeEffects{}
	coordinator := testCoordinator(executor, effects)
	stream, err := coordinator.openSegment(t.Context(), testSegment())
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}

	select {
	case <-executor.cancelStarted:
	case <-time.After(time.Second):
		t.Fatal("CancelTurn did not start")
	}
	if !effects.terminalized("ses_1", "run_1") {
		t.Fatal("CancelTurn started before the synthesized terminal committed")
	}
	close(executor.releaseCancel)
	collectEvents(stream)
}

func TestCoordinatorCommitFailureNeverPublishesUnbackedFact(t *testing.T) {
	executor := &fakeExecutor{events: []EngineEvent{CompactBoundary{MessagesBefore: 4, MessagesAfter: 2}}}
	effects := &fakeEffects{commitErr: fmt.Errorf("store down")}
	coordinator := testCoordinator(executor, effects)

	stream, err := coordinator.openSegment(context.Background(), testSegment())
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	events := collectEvents(stream)
	for _, event := range events {
		if _, ok := event.Payload.(ItemCompleted); ok {
			t.Fatalf("uncommitted item was published: %#v", event.Payload)
		}
		if _, ok := event.Payload.(SegmentFinished); ok {
			t.Fatalf("uncommitted terminal was published: %#v", event.Payload)
		}
	}
	if executor.cancels() != 1 {
		t.Fatalf("CancelTurn calls = %d, want 1", executor.cancels())
	}
}

func TestCoordinatorStartExecutorError(t *testing.T) {
	executor := &fakeExecutor{startErr: fmt.Errorf("boom")}
	coordinator := testCoordinator(executor, &fakeEffects{})

	_, err := coordinator.openSegment(context.Background(), testSegment())
	if err == nil {
		t.Fatal("openSegment must surface the executor error")
	}
	if executor.cancels() != 1 || coordinator.Contains("run_1") {
		t.Fatal("failed executor start was not torn down")
	}
}

func TestCoordinatorPreservesSubscriptionAndCleanupFailures(t *testing.T) {
	startErr := errors.New("event subscription failed")
	cleanupErr := errors.New("executor cleanup failed")
	executor := &fakeExecutor{startErr: startErr, cancelErr: cleanupErr}
	coordinator := testCoordinator(executor, &fakeEffects{})

	_, err := coordinator.openSegment(t.Context(), testSegment())
	if !errors.Is(err, startErr) || !errors.Is(err, cleanupErr) {
		t.Fatalf("openSegment error = %v, want subscription and cleanup failures", err)
	}
}

func TestCoordinatorCloseCancelsAndJoins(t *testing.T) {
	executor := &fakeExecutor{block: true}
	effects := &fakeEffects{rejectCanceled: true}
	coordinator := testCoordinator(executor, effects)
	stream, err := coordinator.openSegment(context.Background(), testSegment())
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	<-stream

	done := make(chan struct{})
	go func() {
		coordinator.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not cancel and join the segment pump")
	}
	collectEvents(stream)
	if !effects.terminalized("ses_1", "run_1") {
		t.Fatal("Close left the run non-terminal after canceling its owner context")
	}
}

func TestCoordinatorStartAfterClose(t *testing.T) {
	executor := &fakeExecutor{}
	coordinator := testCoordinator(executor, &fakeEffects{})
	coordinator.Close()

	_, err := coordinator.openSegment(context.Background(), testSegment())
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("openSegment error = %v, want ErrClosed", err)
	}
	if executor.cancels() != 1 {
		t.Fatalf("CancelTurn calls = %d, want 1", executor.cancels())
	}
}

func TestCoordinatorStartAfterClosePreservesCleanupFailure(t *testing.T) {
	cleanupErr := errors.New("executor cleanup failed")
	executor := &fakeExecutor{cancelErr: cleanupErr}
	coordinator := testCoordinator(executor, &fakeEffects{})
	coordinator.Close()

	_, err := coordinator.openSegment(t.Context(), testSegment())
	if !errors.Is(err, ErrClosed) || !errors.Is(err, cleanupErr) {
		t.Fatalf("openSegment error = %v, want ErrClosed and cleanup failure", err)
	}
}

func TestCoordinatorBeginCancelSurvivesRequestContext(t *testing.T) {
	executor := &fakeExecutor{block: true}
	coordinator := testCoordinator(executor, &fakeEffects{})
	requestContext, cancelRequest := context.WithCancel(context.Background())
	stream, err := coordinator.openSegment(requestContext, testSegment())
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	<-stream
	cancelRequest()

	binding, cleanupContext, cancelCleanup, ok := coordinator.BeginCancel(context.Background(), "run_1", "stop")
	if !ok {
		t.Fatal("BeginCancel did not find the live run")
	}
	defer cancelCleanup()
	if cleanupContext.Err() != nil || binding.SessionID != "ses_1" || binding.TurnID != "turn_1" {
		t.Fatalf("binding=%+v cleanup error=%v", binding, cleanupContext.Err())
	}
	coordinator.Close()
	collectEvents(stream)
}
