package runs

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/admission"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

// These fakes exercise the application-owned reducer and journal. Delivery
// protocol values deliberately do not appear here.
type fakeExecutor struct {
	events         []EngineEvent
	executorEvents []ExecutorEvent
	block          bool
	mu             sync.Mutex
	canceled       int
	startErr       error
	cancelErr      error
	cancelStarted  chan struct{}
	releaseCancel  chan struct{}
}

type acknowledgedChildExecutor struct {
	rootSource   ExecutorSource
	childSource  ExecutorSource
	request      ChildOpeningRequest
	confirmation ChildOpeningConfirmation
	childStarted chan struct{}
}

func (e *acknowledgedChildExecutor) TurnEvents(ctx context.Context, _ execution.TurnRef) (iter.Seq[ExecutorEvent], error) {
	return func(yield func(ExecutorEvent) bool) {
		if !yield(ExecutorEvent{
			Source: e.rootSource,
			Payload: ToolCallStart{
				CallID:       "canonical_call_delegate",
				SourceCallID: e.childSource.SpawnCallID,
				ToolName:     "task",
				Arguments:    `{}`,
			},
		}) {
			return
		}
		if !yield(ExecutorEvent{Source: e.childSource, Payload: e.request}) {
			return
		}
		if err := e.confirmation.Await(ctx); err != nil {
			return
		}
		close(e.childStarted)
		yield(ExecutorEvent{
			Source:  e.rootSource,
			Payload: TurnEnd{Reason: execution.OutcomeCompleted},
		})
	}, nil
}

func (*acknowledgedChildExecutor) CancelTurn(context.Context, execution.TurnRef) error {
	return nil
}

func (f *fakeExecutor) TurnEvents(ctx context.Context, _ execution.TurnRef) (iter.Seq[ExecutorEvent], error) {
	if f.startErr != nil {
		return nil, f.startErr
	}
	return func(yield func(ExecutorEvent) bool) {
		if f.block {
			<-ctx.Done()
			return
		}
		events := f.executorEvents
		if events == nil {
			events = make([]ExecutorEvent, len(f.events))
			for index, event := range f.events {
				events[index] = ExecutorEvent{
					Source:  ExecutorSource{ProcessID: "process_root"},
					Payload: event,
				}
			}
		}
		for _, event := range events {
			if ctx.Err() != nil || !yield(event) {
				return
			}
		}
	}, nil
}

func (f *fakeExecutor) CancelTurn(context.Context, execution.TurnRef) error {
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
	mu              sync.Mutex
	commits         []EventCommit
	openings        []OpeningCommit
	finishes        []Finish
	nudges          int
	openingErr      error
	openingErrAt    int
	commitErr       error
	rejectCanceled  bool
	suspendStarted  chan<- struct{}
	suspendCanceled chan<- struct{}
	suspendRelease  <-chan struct{}
	terminalStarted chan<- struct{}
	terminalRelease <-chan struct{}
	finishStarted   chan<- struct{}
	finishRelease   <-chan struct{}
}

type blockingChildOpeningEffects struct {
	*fakeEffects
	started chan<- struct{}
	release <-chan struct{}
}

func (effects *blockingChildOpeningEffects) CommitOpening(ctx context.Context, opening OpeningCommit) error {
	if opening.Admit != nil && opening.Admit.Lineage().IsChild() {
		effects.started <- struct{}{}
		select {
		case <-effects.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return effects.fakeEffects.CommitOpening(ctx, opening)
}

func (e *fakeEffects) CommitOpening(_ context.Context, opening OpeningCommit) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	attempt := len(e.openings) + 1
	if e.openingErr != nil && (e.openingErrAt == 0 || e.openingErrAt == attempt) {
		return e.openingErr
	}
	e.openings = append(e.openings, opening)
	e.commits = append(e.commits, opening.Events...)
	return nil
}

func (e *fakeEffects) CommitEvent(ctx context.Context, commit EventCommit) error {
	if commit.State == StateSuspend {
		if e.suspendStarted != nil {
			e.suspendStarted <- struct{}{}
		}
		if e.suspendCanceled != nil {
			<-ctx.Done()
			e.suspendCanceled <- struct{}{}
		}
		if e.suspendRelease != nil {
			<-e.suspendRelease
		}
	}
	if commit.State == StateTerminalize {
		if e.terminalStarted != nil {
			e.terminalStarted <- struct{}{}
		}
		if e.terminalRelease != nil {
			<-e.terminalRelease
		}
	}
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

func (e *fakeEffects) openingSnapshot() []OpeningCommit {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]OpeningCommit(nil), e.openings...)
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
		Segments:   executor,
		Effects:    effects,
		Admissions: new(admission.Gate),
		Now: func() time.Time {
			return time.Date(2026, 7, 13, 1, 2, 3, 0, time.UTC)
		},
	})
}

func testSegment() segmentSpec {
	return segmentSpec{
		RunID: "run_1", SegmentID: "seg_1", SessionID: "ses_1",
		TurnID: "turn_1", ModelSelection: mustSelection("openai", "model"),
		CreatedAt: time.Date(2026, 7, 13, 1, 2, 3, 0, time.UTC),
	}
}

func mustSelection(provider, model string) modelref.Selection {
	selection, err := modelref.New(provider, model)
	if err != nil {
		panic(err)
	}
	return selection
}

func testAdmittedSegment(t *testing.T, c *Coordinator, spec segmentSpec) segmentSpec {
	t.Helper()
	admission, ok := c.admission.AcquireRun(spec.SessionID, spec.Cwd)
	if !ok {
		t.Fatal("acquire test run admission")
	}
	spec.admission = &admission
	return spec
}

func collectEvents(events iter.Seq[Event]) []Event {
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
	if _, ok := coordinator.registry.Get("run_1"); events != nil || ok {
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
	spec.Input = []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "question"}}

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
	// The opening's durable projection is the admission plus the user item; the
	// SegmentStarted event carries no second copy of the Run.
	if opening := effects.opening(); opening.Admit == nil || opening.Resume != nil || len(opening.Events) != 1 {
		t.Fatalf("opening = %+v, want admit + the user-item commit", opening)
	}
	if !effects.terminalized("ses_1", "run_1") {
		t.Fatal("terminal run and exact run-state transition were not committed")
	}
	for index := 1; index < len(events); index++ {
		if events[index-1].Sequence >= events[index].Sequence {
			t.Fatalf("stream positions are not monotonic: %d then %d", events[index-1].Sequence, events[index].Sequence)
		}
	}
}

func TestCoordinatorHoldsSessionAdmissionThroughTerminalMaintenance(t *testing.T) {
	executor := &fakeExecutor{events: []EngineEvent{TurnEnd{Reason: execution.OutcomeCompleted}}}
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	effects := &fakeEffects{finishStarted: started, finishRelease: release}
	coordinator := testCoordinator(executor, effects)

	stream, err := coordinator.openSegment(t.Context(), testAdmittedSegment(t, coordinator, testSegment()))
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	streamDone := make(chan []Event, 1)
	go func() { streamDone <- collectEvents(stream) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("terminal maintenance did not start")
	}
	if _, ok := coordinator.registry.Get("run_1"); !ok {
		t.Fatal("terminal maintenance removed the run's cancellation join identity")
	}
	if !hasActiveSession(coordinator, "ses_1") {
		t.Fatal("session admission was released before terminal maintenance completed")
	}
	if _, ok := coordinator.admission.AcquireSession("ses_1"); ok {
		t.Fatal("new run admission crossed the terminal-maintenance fence")
	}
	select {
	case <-streamDone:
		t.Fatal("stream closed before terminal maintenance released admission")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	select {
	case <-streamDone:
	case <-time.After(time.Second):
		t.Fatal("stream did not close after terminal maintenance released admission")
	}
	requireCoordinatorShutdown(t, coordinator)
	if hasActiveSession(coordinator, "ses_1") {
		t.Fatal("terminal-maintenance claim was not released")
	}
}

func hasActiveSession(c *Coordinator, sessionID string) bool {
	return c.admission.ActiveSessions()[sessionID]
}

func TestCoordinatorCommitsProcessCreationFailureInCanonicalOrder(t *testing.T) {
	executor := &fakeExecutor{events: []EngineEvent{
		TurnEnd{Reason: execution.OutcomeError, Problem: &transcript.Problem{Kind: transcript.InternalProblem, Scope: transcript.RunProblem, Detail: "the run failed due to an internal error"}},
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
	if finished.Run.Error == nil || finished.Run.Error.Kind != transcript.InternalProblem {
		t.Fatalf("run failure = %+v, want canonical internal problem", finished.Run.Error)
	}
	if events[0].Sequence >= events[1].Sequence {
		t.Fatalf("event order = %d then %d, want monotonic", events[0].Sequence, events[1].Sequence)
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
	if finished.Run.Error == nil {
		t.Fatalf("error terminal has no canonical problem: %+v", finished.Run)
	}
}

func TestCoordinatorMalformedInterruptAbortsExecutorAndTerminalizes(t *testing.T) {
	executor := &fakeExecutor{events: []EngineEvent{TurnInterrupted{Interrupts: []Interrupt{{Kind: execution.InterruptKind(9)}}}}}
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
			if finished.Run.Error == nil || finished.Run.Error.Kind != transcript.InternalProblem {
				t.Fatalf("run failure = %+v, want canonical internal problem", finished.Run.Error)
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

func TestCoordinatorRejectsUnadmittedChildSource(t *testing.T) {
	executor := &fakeExecutor{executorEvents: []ExecutorEvent{{
		Source: ExecutorSource{
			ProcessID:   "process_child",
			ParentID:    "process_root",
			SpawnCallID: "call_delegate",
		},
		Payload: MessageDelta{Text: "must not reach the root reducer"},
	}}}
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
	if executor.cancels() != 1 {
		t.Fatalf("CancelTurn calls = %d, want 1", executor.cancels())
	}
	if !effects.terminalized("ses_1", "run_1") {
		t.Fatal("unadmitted child source did not terminalize the root run")
	}
}

func TestCoordinatorAtomicallyAdmitsChildRunFromSpawningItem(t *testing.T) {
	startedAt := time.Date(2026, 7, 13, 1, 2, 4, 0, time.FixedZone("test", 8*60*60))
	request, confirmation := NewChildOpeningRequest(startedAt)
	rootSource := ExecutorSource{ProcessID: "process_root"}
	childSource := ExecutorSource{
		ProcessID:   "process_child",
		ParentID:    rootSource.ProcessID,
		SpawnCallID: "provider_call_delegate",
	}
	executor := &fakeExecutor{executorEvents: []ExecutorEvent{
		{
			Source: rootSource,
			Payload: ToolCallStart{
				CallID:       "canonical_call_delegate",
				SourceCallID: childSource.SpawnCallID,
				ToolName:     "task",
				Arguments:    `{"description":"delegate"}`,
			},
		},
		{Source: childSource, Payload: request},
		{Source: rootSource, Payload: TurnEnd{Reason: execution.OutcomeCompleted}},
	}}
	effects := &fakeEffects{}
	coordinator := testCoordinator(executor, effects)
	coordinator.newRunID = func() string { return "run_child" }
	coordinator.newSegmentID = func() string { return "seg_child" }

	stream, err := coordinator.openSegment(t.Context(), testSegment())
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	collectEvents(stream)
	if err := confirmation.Await(t.Context()); err != nil {
		t.Fatalf("child opening confirmation: %v", err)
	}

	openings := effects.openingSnapshot()
	if len(openings) != 2 {
		t.Fatalf("opening commits = %d, want root and child", len(openings))
	}
	child := openings[1]
	if child.Admit == nil {
		t.Fatal("child opening has no admission draft")
	}
	draft := *child.Admit
	if draft.RunID != "run_child" ||
		draft.SessionID != "ses_1" ||
		draft.SpawnedByItemID != "item_seg_1_1" ||
		draft.ParentRunID != "run_1" ||
		draft.RootRunID != "run_1" ||
		draft.SegmentID != "seg_child" ||
		draft.ModelSelection != testSegment().ModelSelection ||
		!draft.ProtocolProfile.IsEmpty() ||
		!draft.CreatedAt.Equal(startedAt) {
		t.Fatalf("child admission draft = %+v", draft)
	}
	if len(child.Events) != 1 || len(child.Events[0].Items) != 1 {
		t.Fatalf("child opening events = %+v, want parent spawning item", child.Events)
	}
	parentCommit := child.Events[0]
	spawningItem := parentCommit.Items[0]
	if parentCommit.RunID != "run_1" ||
		parentCommit.SessionID != "ses_1" ||
		spawningItem.ID != draft.SpawnedByItemID ||
		spawningItem.RunID != "run_1" ||
		spawningItem.SessionID != "ses_1" ||
		spawningItem.Status != transcript.ItemRunning ||
		spawningItem.Tool == nil ||
		spawningItem.Tool.Name != "task" {
		t.Fatalf("parent spawning-item commit = %+v", parentCommit)
	}
}

func TestCoordinatorRejectsChildWhenAtomicOpeningFails(t *testing.T) {
	commitErr := errors.New("child opening transaction failed")
	request, confirmation := NewChildOpeningRequest(time.Now())
	rootSource := ExecutorSource{ProcessID: "process_root"}
	childSource := ExecutorSource{
		ProcessID:   "process_child",
		ParentID:    rootSource.ProcessID,
		SpawnCallID: "provider_call_delegate",
	}
	executor := &fakeExecutor{executorEvents: []ExecutorEvent{
		{
			Source: rootSource,
			Payload: ToolCallStart{
				CallID:       "canonical_call_delegate",
				SourceCallID: childSource.SpawnCallID,
				ToolName:     "task",
				Arguments:    `{}`,
			},
		},
		{Source: childSource, Payload: request},
	}}
	effects := &fakeEffects{openingErr: commitErr, openingErrAt: 2}
	coordinator := testCoordinator(executor, effects)
	coordinator.newRunID = func() string { return "run_child" }
	coordinator.newSegmentID = func() string { return "seg_child" }

	stream, err := coordinator.openSegment(t.Context(), testSegment())
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	events := collectEvents(stream)
	if err := confirmation.Await(t.Context()); !errors.Is(err, commitErr) {
		t.Fatalf("child opening confirmation = %v, want commit failure", err)
	}
	if openings := effects.openingSnapshot(); len(openings) != 1 {
		t.Fatalf("committed openings = %d, want only root", len(openings))
	}
	finished, ok := events[len(events)-1].Payload.(SegmentFinished)
	if !ok || finished.Run.Outcome == nil || *finished.Run.Outcome != execution.OutcomeError {
		t.Fatalf("last payload = %#v, want root error terminal", events[len(events)-1].Payload)
	}
	if !effects.terminalized("ses_1", "run_1") {
		t.Fatal("failed child opening did not terminalize the root")
	}
	if executor.cancels() != 1 {
		t.Fatalf("CancelTurn calls = %d, want 1", executor.cancels())
	}
}

func TestCoordinatorAcknowledgesChildOnlyAfterOpeningCommit(t *testing.T) {
	request, confirmation := NewChildOpeningRequest(time.Now())
	rootSource := ExecutorSource{ProcessID: "process_root"}
	childSource := ExecutorSource{
		ProcessID:   "process_child",
		ParentID:    rootSource.ProcessID,
		SpawnCallID: "provider_call_delegate",
	}
	executor := &acknowledgedChildExecutor{
		rootSource:   rootSource,
		childSource:  childSource,
		request:      request,
		confirmation: confirmation,
		childStarted: make(chan struct{}),
	}
	commitStarted := make(chan struct{}, 1)
	releaseCommit := make(chan struct{})
	baseEffects := &fakeEffects{}
	effects := &blockingChildOpeningEffects{
		fakeEffects: baseEffects,
		started:     commitStarted,
		release:     releaseCommit,
	}
	coordinator := testCoordinator(executor, effects)
	coordinator.newRunID = func() string { return "run_child" }
	coordinator.newSegmentID = func() string { return "seg_child" }

	stream, err := coordinator.openSegment(t.Context(), testSegment())
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	drained := make(chan struct{})
	go func() {
		collectEvents(stream)
		close(drained)
	}()

	select {
	case <-commitStarted:
	case <-time.After(time.Second):
		t.Fatal("child opening commit did not start")
	}
	select {
	case <-executor.childStarted:
		t.Fatal("executor resumed child before its opening commit completed")
	default:
	}
	close(releaseCommit)
	select {
	case <-executor.childStarted:
	case <-time.After(time.Second):
		t.Fatal("executor did not resume child after opening commit")
	}
	select {
	case <-drained:
	case <-time.After(time.Second):
		t.Fatal("root stream did not finish after child admission")
	}
	if openings := baseEffects.openingSnapshot(); len(openings) != 2 {
		t.Fatalf("opening commits = %d, want root and child", len(openings))
	}
}

func TestCoordinatorCommitsSyntheticTerminalBeforeCancelTurn(t *testing.T) {
	executor := &fakeExecutor{
		events:        []EngineEvent{TurnInterrupted{Interrupts: []Interrupt{{Kind: execution.InterruptKind(9)}}}},
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
	if _, ok := coordinator.registry.Get("run_1"); executor.cancels() != 1 || ok {
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
	next, stop := iter.Pull(stream)
	defer stop()
	next() // consume the opening event so the pump is live

	done := make(chan struct{})
	go func() {
		_ = shutdownCoordinator(coordinator)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not cancel and join the segment pump")
	}
	for _, ok := next(); ok; _, ok = next() { // drain the remaining terminal events
	}
	if !effects.terminalized("ses_1", "run_1") {
		t.Fatal("Close left the run non-terminal after canceling its owner context")
	}
}

func TestCoordinatorStartAfterClose(t *testing.T) {
	executor := &fakeExecutor{}
	coordinator := testCoordinator(executor, &fakeEffects{})
	requireCoordinatorShutdown(t, coordinator)

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
	requireCoordinatorShutdown(t, coordinator)

	_, err := coordinator.openSegment(t.Context(), testSegment())
	if !errors.Is(err, ErrClosed) || !errors.Is(err, cleanupErr) {
		t.Fatalf("openSegment error = %v, want ErrClosed and cleanup failure", err)
	}
}

func TestCoordinatorCancelContextSurvivesRequestContext(t *testing.T) {
	executor := &fakeExecutor{block: true}
	coordinator := testCoordinator(executor, &fakeEffects{})
	turns := &fakeTurnControl{}
	coordinator.turns = turns
	coordinator.sessions = &fakeRunSessions{}
	coordinator.runs = &fakeRunProjection{runs: map[string]transcript.Run{
		"run_1": {ID: "run_1", SessionID: "ses_1", State: execution.Running, ActiveSegmentID: "seg_1"},
	}}
	requestContext, cancelRequest := context.WithCancel(context.Background())
	stream, err := coordinator.openSegment(requestContext, testSegment())
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	next, stop := iter.Pull(stream)
	defer stop()
	next() // consume the opening event so the pump is live
	cancelRequest()

	if _, err := coordinator.Cancel(requestContext, CancelCommand{RunID: "run_1", Reason: "stop"}); err != nil {
		t.Fatalf("Cancel with finished request context: %v", err)
	}
	if executor.cancels() != 1 {
		t.Fatalf("pump executor cancellations = %d, want one", executor.cancels())
	}
	if len(turns.canceled) != 0 {
		t.Fatalf("control-surface cancellations = %+v, want pump-only ownership", turns.canceled)
	}
	requireCoordinatorShutdown(t, coordinator)
	for _, ok := next(); ok; _, ok = next() { // drain whatever remains
	}
}
