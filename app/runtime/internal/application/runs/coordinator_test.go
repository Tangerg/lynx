package runs

import (
	"context"
	"errors"
	"iter"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/admission"
	"github.com/Tangerg/lynx/app/runtime/internal/component/replaycursor"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

func mustTreeContinuation(t *testing.T, pending interrupts.Pending) *treeContinuation {
	t.Helper()
	continuation, err := treeContinuationFromPending(pending)
	if err != nil {
		t.Fatalf("tree continuation: %v", err)
	}
	return continuation
}

func testTreeContinuation(pending interrupts.Pending) *treeContinuation {
	return &treeContinuation{
		rootRunID:     pending.RootRunID,
		sessionID:     pending.SessionID,
		turnID:        pending.TurnID,
		goalLeaseID:   pending.GoalLeaseID,
		interrupts:    slices.Clone(pending.Interrupts),
		continuations: slices.Clone(pending.Continuations),
		profile:       pending.ProtocolProfile,
	}
}

// These fakes exercise the application-owned reducer and journal. Delivery
// protocol values deliberately do not appear here.
type fakeExecutor struct {
	events         []ExecutorPayload
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

type cancellableChildExecutor struct {
	rootSource      ExecutorSource
	childSource     ExecutorSource
	request         ChildOpeningRequest
	confirmation    ChildOpeningConfirmation
	childOpened     chan struct{}
	cancelRequested chan struct{}
	finishRoot      chan struct{}
}

func (e *cancellableChildExecutor) TurnEvents(
	ctx context.Context,
	_ execution.TurnRef,
) (iter.Seq[ExecutorEvent], error) {
	return func(yield func(ExecutorEvent) bool) {
		if !yield(ExecutorEvent{
			Source: e.rootSource,
			Payload: ToolCallStart{
				CallID:       "canonical_child",
				SourceCallID: e.childSource.SpawnCallID,
				ToolName:     "delegate_task",
				Arguments:    `{}`,
			},
		}) {
			return
		}
		if !yield(ExecutorEvent{Source: e.childSource, Payload: e.request}) {
			return
		}
		if _, err := e.confirmation.Await(ctx); err != nil {
			return
		}
		close(e.childOpened)
		select {
		case <-e.cancelRequested:
		case <-ctx.Done():
			return
		}
		if !yield(ExecutorEvent{
			Source:  e.childSource,
			Payload: TurnEnd{Reason: execution.OutcomeCanceled},
		}) {
			return
		}
		if !yield(ExecutorEvent{
			Source: e.rootSource,
			Payload: ToolCallEnd{
				CallID: "canonical_child",
				Problem: &transcript.Problem{
					Kind:   transcript.ToolFailedProblem,
					Scope:  transcript.ToolProblem,
					Detail: "executor process was killed",
				},
			},
		}) {
			return
		}
		select {
		case <-e.finishRoot:
		case <-ctx.Done():
			return
		}
		yield(ExecutorEvent{
			Source:  e.rootSource,
			Payload: TurnEnd{Reason: execution.OutcomeCompleted},
		})
	}, nil
}

func (*cancellableChildExecutor) CancelTurn(context.Context, execution.TurnRef) error {
	return nil
}

func (e *acknowledgedChildExecutor) TurnEvents(ctx context.Context, _ execution.TurnRef) (iter.Seq[ExecutorEvent], error) {
	return func(yield func(ExecutorEvent) bool) {
		if !yield(ExecutorEvent{
			Source: e.rootSource,
			Payload: ToolCallStart{
				CallID:       "canonical_call_delegate",
				SourceCallID: e.childSource.SpawnCallID,
				ToolName:     "delegate_task",
				Arguments:    `{}`,
			},
		}) {
			return
		}
		if !yield(ExecutorEvent{Source: e.childSource, Payload: e.request}) {
			return
		}
		if _, err := e.confirmation.Await(ctx); err != nil {
			return
		}
		close(e.childStarted)
		if !yield(ExecutorEvent{
			Source:  e.childSource,
			Payload: TurnEnd{Reason: execution.OutcomeCompleted},
		}) {
			return
		}
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
	barriers        []TreeBarrierCommit
	waitingCancels  []WaitingSubtreeCancellationCommit
	waitingResult   WaitingSubtreeCancellationResult
	waitingErr      error
	finishes        []Finish
	nudges          int
	openingErr      error
	openingErrAt    int
	commitErr       error
	commitErrAt     int
	commitAttempts  int
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
	e.commitAttempts++
	if e.rejectCanceled && ctx.Err() != nil {
		return ctx.Err()
	}
	if e.commitErr != nil && (e.commitErrAt == 0 || e.commitErrAt == e.commitAttempts) {
		return e.commitErr
	}
	e.commits = append(e.commits, commit)
	return nil
}

func (e *fakeEffects) CommitTreeBarrier(ctx context.Context, barrier TreeBarrierCommit) error {
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
	e.mu.Lock()
	defer e.mu.Unlock()
	e.commitAttempts++
	if e.rejectCanceled && ctx.Err() != nil {
		return ctx.Err()
	}
	if e.commitErr != nil && (e.commitErrAt == 0 || e.commitErrAt == e.commitAttempts) {
		return e.commitErr
	}
	e.barriers = append(e.barriers, barrier)
	e.commits = append(e.commits, barrier.Runs...)
	return nil
}

func (e *fakeEffects) CommitWaitingSubtreeCancellation(
	_ context.Context,
	commit WaitingSubtreeCancellationCommit,
) (WaitingSubtreeCancellationResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.waitingErr != nil {
		return WaitingSubtreeCancellationResult{}, e.waitingErr
	}
	e.waitingCancels = append(e.waitingCancels, commit)
	return e.waitingResult, nil
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

func (e *fakeEffects) commitSnapshot() []EventCommit {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]EventCommit(nil), e.commits...)
}

func (e *fakeEffects) barrierSnapshot() []TreeBarrierCommit {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]TreeBarrierCommit(nil), e.barriers...)
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

func runForSegment(spec segmentSpec) transcript.Run {
	return transcript.Run{
		ID: spec.RunID, SessionID: spec.SessionID, State: execution.Running,
		ActiveSegmentID: spec.SegmentID, ModelSelection: spec.ModelSelection,
		GoalLeaseID: spec.GoalLeaseID, Limits: spec.Limits,
		ProtocolProfile: spec.ProtocolProfile,
		CreatedAt:       spec.CreatedAt, UpdatedAt: spec.CreatedAt,
		MessageMark: transcript.UnknownMessageMark,
	}
}

func TestResumedExecutorRouteRetainsGoalLeaseForTerminalAccounting(t *testing.T) {
	createdAt := time.Date(2026, 7, 30, 1, 2, 3, 0, time.UTC)
	pending := testPendingInterrupt("item_1", "process_root", createdAt)
	pending.GoalLeaseID = "goal-lease-1"
	pending.Continuations[0].ModelSelection = mustSelection("openai", "model")
	continuation := mustTreeContinuation(t, pending)
	spec := testSegment()
	spec.Continuation = continuation
	spec.GoalLeaseID = pending.GoalLeaseID

	routes, err := testCoordinator(&fakeExecutor{}, &fakeEffects{}).resumedExecutorRoutes(spec, nil)
	if err != nil {
		t.Fatalf("resumedExecutorRoutes: %v", err)
	}
	if routes.root.reducer.cfg.GoalLeaseID != pending.GoalLeaseID {
		t.Fatalf(
			"resumed reducer goal lease = %q, want %q",
			routes.root.reducer.cfg.GoalLeaseID,
			pending.GoalLeaseID,
		)
	}
}

func TestResumedExecutorRoutesBindLiveTopologyWithoutPersistingIt(t *testing.T) {
	createdAt := time.Date(2026, 7, 30, 1, 2, 3, 0, time.UTC)
	pending := resumedTreePending(createdAt)
	continuation := mustTreeContinuation(t, pending)
	spec := testSegment()
	spec.RunID = pending.RootRunID
	spec.SessionID = pending.SessionID
	spec.Continuation = continuation

	coordinator := testCoordinator(&fakeExecutor{}, &fakeEffects{})
	segmentIDs := []string{"segment_grandchild", "segment_a", "segment_b"}
	coordinator.newSegmentID = func() string {
		segmentID := segmentIDs[0]
		segmentIDs = segmentIDs[1:]
		return segmentID
	}
	routes, err := coordinator.resumedExecutorRoutes(spec, nil)
	if err != nil {
		t.Fatalf("resumedExecutorRoutes: %v", err)
	}
	child := routes.byRunID["run_a"]
	if child == nil || child.sourceBound || child.source.ProcessID != "process_a" {
		t.Fatalf("restored child route = %+v, want opaque process binding without persisted topology", child)
	}

	source := ExecutorSource{ProcessID: "process_a", ParentID: "process_root", SpawnCallID: "spawn_a"}
	resolved, err := routes.resolve(source)
	if err != nil || resolved != child || !child.sourceBound || child.source != source {
		t.Fatalf("resolve live child source = (%+v, %v), route=%+v", resolved, err, child)
	}
	if _, err := routes.resolve(ExecutorSource{
		ProcessID: "process_a", ParentID: "process_root", SpawnCallID: "changed",
	}); err == nil || !strings.Contains(err.Error(), "changed immutable lineage") {
		t.Fatalf("changed live child topology error = %v", err)
	}

	if _, err := routes.resolve(ExecutorSource{
		ProcessID: "process_b", ParentID: "process_a", SpawnCallID: "spawn_b",
	}); err == nil || !strings.Contains(err.Error(), "want Run") {
		t.Fatalf("wrong live parent error = %v", err)
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

func treeBarrierQuestion(prompt string) Interrupt {
	return Interrupt{
		Kind: execution.QuestionInterrupt,
		Question: &QuestionPrompt{
			ToolName:  "ask_user",
			Arguments: `{}`,
			Fields:    []QuestionFieldSpec{{Prompt: prompt, Header: "Decision"}},
		},
	}
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
	executor := &fakeExecutor{events: []ExecutorPayload{
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
	executor := &fakeExecutor{events: []ExecutorPayload{TurnEnd{Reason: execution.OutcomeCompleted}}}
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
	executor := &fakeExecutor{events: []ExecutorPayload{
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
	pending := testPendingInterrupt("item_1", "process_root", spec.CreatedAt)
	spec.Continuation = mustTreeContinuation(t, pending)
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
	if opening.Resume == nil || opening.Resume.RootRunID != "run_1" || opening.Admit != nil {
		t.Fatalf("opening = %+v, want resume run_1", opening)
	}
}

func TestCoordinatorResumesCompleteRunTreeInOneCanonicalOpening(t *testing.T) {
	rootSource := ExecutorSource{ProcessID: "process_root"}
	childASource := ExecutorSource{
		ProcessID: "process_a", ParentID: "process_root", SpawnCallID: "spawn_a",
	}
	grandchildSource := ExecutorSource{
		ProcessID: "process_grandchild", ParentID: "process_a", SpawnCallID: "spawn_grandchild",
	}
	childBSource := ExecutorSource{
		ProcessID: "process_b", ParentID: "process_root", SpawnCallID: "spawn_b",
	}
	executor := &fakeExecutor{executorEvents: []ExecutorEvent{
		{Source: grandchildSource, Payload: TurnEnd{Reason: execution.OutcomeCompleted}},
		{Source: childASource, Payload: TurnEnd{Reason: execution.OutcomeCompleted}},
		{Source: childBSource, Payload: TurnEnd{Reason: execution.OutcomeCompleted}},
		{Source: rootSource, Payload: TurnEnd{Reason: execution.OutcomeCompleted}},
	}}
	effects := &fakeEffects{}
	coordinator := testCoordinator(executor, effects)
	childSegmentIDs := []string{"seg_grandchild", "seg_a", "seg_b"}
	coordinator.newSegmentID = func() string {
		next := childSegmentIDs[0]
		childSegmentIDs = childSegmentIDs[1:]
		return next
	}
	spec := testSegment()
	spec.SegmentID = "seg_root_resumed"
	pending := resumedTreePending(spec.CreatedAt)
	spec.Continuation = mustTreeContinuation(t, pending)

	stream, err := coordinator.openSegment(t.Context(), spec)
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	events := collectEvents(stream)
	opening := effects.opening()
	if opening.Resume == nil {
		t.Fatal("opening has no tree resume draft")
	}
	wantRuns := []execution.RunResumeDraft{
		{RunID: "run_grandchild", SegmentID: "seg_grandchild"},
		{RunID: "run_a", SegmentID: "seg_a"},
		{RunID: "run_b", SegmentID: "seg_b"},
		{RunID: "run_1", SegmentID: "seg_root_resumed"},
	}
	if !slices.Equal(opening.Resume.Runs, wantRuns) {
		t.Fatalf("tree resume Runs = %#v, want %#v", opening.Resume.Runs, wantRuns)
	}

	var started, finished []string
	for _, event := range events {
		switch payload := event.Payload.(type) {
		case SegmentStarted:
			started = append(started, event.RunID)
			if payload.Run.ActiveSegmentID != event.SegmentID {
				t.Fatalf(
					"Run %q active segment = %q, envelope = %q",
					event.RunID,
					payload.Run.ActiveSegmentID,
					event.SegmentID,
				)
			}
		case SegmentFinished:
			finished = append(finished, event.RunID)
		}
	}
	wantPostorder := []string{"run_grandchild", "run_a", "run_b", "run_1"}
	if !slices.Equal(started, wantPostorder) {
		t.Fatalf("SegmentStarted order = %v, want %v", started, wantPostorder)
	}
	if !slices.Equal(finished, wantPostorder) {
		t.Fatalf("SegmentFinished order = %v, want %v", finished, wantPostorder)
	}
	var checkpointDeletes []string
	for _, commit := range effects.commitSnapshot() {
		if commit.ObsoleteCheckpointRootID != "" {
			checkpointDeletes = append(checkpointDeletes, commit.ObsoleteCheckpointRootID)
		}
	}
	if !slices.Equal(checkpointDeletes, []string{rootSource.ProcessID}) {
		t.Fatalf("terminal executor checkpoint deletes = %v, want root only", checkpointDeletes)
	}
}

func resumedTreePending(createdAt time.Time) interrupts.Pending {
	question := func(itemID, runID string) transcript.Interrupt {
		return transcript.Interrupt{
			ItemID: itemID, ItemOccurredAt: createdAt,
			RunID: runID, Kind: execution.QuestionInterrupt,
			Question: &transcript.Question{
				Fields: []transcript.QuestionField{{
					Prompt: "Continue?", Kind: transcript.QuestionText,
				}},
			},
		}
	}
	return interrupts.Pending{
		RootRunID: "run_1",
		SessionID: "ses_1",
		TurnID:    "turn_1",
		ProtocolProfile: execution.RunProtocolProfile{
			ChildRuns:      true,
			InterruptKinds: []execution.InterruptKind{execution.QuestionInterrupt},
		},
		Interrupts: []transcript.Interrupt{
			question("item_grandchild", "run_grandchild"),
			question("item_b", "run_b"),
		},
		Suspensions: []interrupts.SuspensionBinding{
			{InterruptItemID: "item_grandchild", ProcessID: "process_grandchild", SuspensionID: "suspension_grandchild"},
			{InterruptItemID: "item_b", ProcessID: "process_b", SuspensionID: "suspension_b"},
		},
		Continuations: []interrupts.Continuation{
			{
				RunID:     "run_grandchild",
				ProcessID: "process_grandchild",
				Lineage: execution.RunLineage{
					SpawnedByItemID: "item_spawn_grandchild",
					ParentRunID:     "run_a",
					RootRunID:       "run_1",
				},
				ModelSelection: mustSelection("openai", "model"),
				RunCreatedAt:   createdAt,
			},
			{
				RunID:     "run_a",
				ProcessID: "process_a",
				Lineage: execution.RunLineage{
					SpawnedByItemID: "item_spawn_a",
					ParentRunID:     "run_1",
					RootRunID:       "run_1",
				},
				ModelSelection: mustSelection("openai", "model"),
				RunCreatedAt:   createdAt,
			},
			{
				RunID:     "run_b",
				ProcessID: "process_b",
				Lineage: execution.RunLineage{
					SpawnedByItemID: "item_spawn_b",
					ParentRunID:     "run_1",
					RootRunID:       "run_1",
				},
				ModelSelection: mustSelection("openai", "model"),
				RunCreatedAt:   createdAt,
			},
			{
				RunID:          "run_1",
				ProcessID:      "process_root",
				ModelSelection: mustSelection("openai", "model"),
				RunCreatedAt:   createdAt,
			},
		},
		CreatedAt: createdAt.Add(time.Second),
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

func TestCoordinatorTreeActivationFailureTerminalizesInCanonicalPostorder(t *testing.T) {
	executor := &fakeExecutor{block: true}
	effects := &fakeEffects{}
	coordinator := testCoordinator(executor, effects)
	childSegmentIDs := []string{"seg_grandchild", "seg_a", "seg_b"}
	coordinator.newSegmentID = func() string {
		next := childSegmentIDs[0]
		childSegmentIDs = childSegmentIDs[1:]
		return next
	}
	spec := testSegment()
	spec.SegmentID = "seg_root_resumed"
	pending := resumedTreePending(spec.CreatedAt)
	spec.Continuation = mustTreeContinuation(t, pending)
	spec.Activate = func(context.Context) error { return errors.New("resume failed") }

	stream, err := coordinator.openSegment(t.Context(), spec)
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	events := collectEvents(stream)
	var started, finished []string
	for _, event := range events {
		switch payload := event.Payload.(type) {
		case SegmentStarted:
			started = append(started, event.RunID)
		case SegmentFinished:
			finished = append(finished, event.RunID)
			if payload.Run.Outcome == nil || *payload.Run.Outcome != execution.OutcomeError {
				t.Fatalf("Run %q outcome = %v, want error", event.RunID, payload.Run.Outcome)
			}
		}
	}
	wantPostorder := []string{"run_grandchild", "run_a", "run_b", "run_1"}
	if !slices.Equal(started, wantPostorder) {
		t.Fatalf("SegmentStarted order = %v, want %v", started, wantPostorder)
	}
	if !slices.Equal(finished, wantPostorder) {
		t.Fatalf("SegmentFinished order = %v, want %v", finished, wantPostorder)
	}
}

func TestCoordinatorMalformedInterruptAbortsExecutorAndTerminalizes(t *testing.T) {
	executor := &fakeExecutor{events: []ExecutorPayload{TurnInterrupted{Interrupts: []Interrupt{{Kind: execution.InterruptKind(9)}}}}}
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
			executor := &fakeExecutor{events: []ExecutorPayload{test.event}}
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
				ToolName:     "delegate_task",
				Arguments:    `{"description":"delegate"}`,
			},
		},
		{Source: childSource, Payload: request},
		{Source: childSource, Payload: TurnEnd{Reason: execution.OutcomeCompleted}},
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
	binding, err := confirmation.Await(t.Context())
	if err != nil {
		t.Fatalf("child opening confirmation: %v", err)
	}
	wantBinding := ChildRunBinding{
		ProcessID: childSource.ProcessID, RunID: "run_child", ParentRunID: testSegment().RunID,
	}
	if binding != wantBinding {
		t.Fatalf("child opening binding = %+v, want %+v", binding, wantBinding)
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
		spawningItem.Tool.Name != "delegate_task" {
		t.Fatalf("parent spawning-item commit = %+v", parentCommit)
	}
}

func TestCoordinatorPublishesChildSegmentOnItsOwnRunIdentity(t *testing.T) {
	request, confirmation := NewChildOpeningRequest(time.Date(2026, 7, 13, 1, 2, 4, 0, time.UTC))
	rootSource := ExecutorSource{ProcessID: "process_root"}
	childSource := ExecutorSource{
		ProcessID:   "process_child",
		ParentID:    rootSource.ProcessID,
		SpawnCallID: "provider_call_delegate",
	}
	finalUsage := TurnUsage{
		Tokens: accounting.TokenUsage{
			PromptTokens:     13,
			CompletionTokens: 5,
		},
		ByModel: []accounting.ModelUsage{{
			Model: "child-model",
			TokenUsage: accounting.TokenUsage{
				PromptTokens:     13,
				CompletionTokens: 5,
			},
			Calls: 1,
		}},
		Steps: 1,
	}
	executor := &fakeExecutor{executorEvents: []ExecutorEvent{
		{
			Source: rootSource,
			Payload: ToolCallStart{
				CallID:       "canonical_call_delegate",
				SourceCallID: childSource.SpawnCallID,
				ToolName:     "delegate_task",
				Arguments:    `{"description":"delegate"}`,
			},
		},
		{Source: childSource, Payload: request},
		{Source: childSource, Payload: MessageDelta{Text: "child reply"}},
		{Source: childSource, Payload: TurnEnd{
			Reason: execution.OutcomeCompleted,
			Usage:  &finalUsage,
		}},
		{Source: rootSource, Payload: ToolCallEnd{
			CallID:     "canonical_call_delegate",
			OutputText: "child reply",
		}},
		{Source: rootSource, Payload: TurnEnd{Reason: execution.OutcomeCompleted}},
	}}
	effects := &fakeEffects{}
	coordinator := testCoordinator(executor, effects)
	coordinator.newRunID = func() string { return "run_child" }
	coordinator.newSegmentID = func() string { return "seg_child" }
	spec := testSegment()
	spec.Limits = execution.RunLimits{MaxSteps: 20, MaxBudgetUSD: 3}
	spec.ProtocolProfile = execution.RunProtocolProfile{
		ChildRuns: true,
	}

	stream, err := coordinator.openSegment(t.Context(), spec)
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	events := collectEvents(stream)
	if _, err := confirmation.Await(t.Context()); err != nil {
		t.Fatalf("child opening confirmation: %v", err)
	}

	var (
		childStarted   *SegmentStarted
		childCompleted *ItemCompleted
		childFinished  *SegmentFinished
	)
	for index := range events {
		event := events[index]
		switch payload := event.Payload.(type) {
		case SegmentStarted:
			if event.RunID == "run_child" {
				childStarted = &payload
			}
		case ItemCompleted:
			if event.RunID == "run_child" {
				childCompleted = &payload
			}
		case SegmentFinished:
			if event.RunID == "run_child" {
				childFinished = &payload
			}
		}
		if event.RunID == "run_child" && event.SegmentID != "seg_child" {
			t.Fatalf("child event[%d] segment = %q, want seg_child", index, event.SegmentID)
		}
	}
	if childStarted == nil {
		t.Fatal("root Journal did not publish child segment.started")
	}
	lineage := execution.RunLineage{
		SpawnedByItemID: "item_seg_1_1",
		ParentRunID:     "run_1",
		RootRunID:       "run_1",
	}
	if childStarted.Run.Lineage() != lineage ||
		childStarted.Run.ActiveSegmentID != "seg_child" ||
		childStarted.Run.Limits != spec.Limits ||
		childStarted.Run.ProtocolProfile.String() != spec.ProtocolProfile.String() {
		t.Fatalf("child opening run = %+v, want independent inherited segment state", childStarted.Run)
	}
	if childCompleted == nil ||
		childCompleted.Item.RunID != "run_child" ||
		childCompleted.Item.SessionID != spec.SessionID ||
		childCompleted.Item.Kind != transcript.AgentMessage {
		t.Fatalf("child completed item = %+v, want child-owned assistant item", childCompleted)
	}
	if childFinished == nil ||
		childFinished.Run.Lineage() != lineage ||
		childFinished.Run.Outcome == nil ||
		*childFinished.Run.Outcome != execution.OutcomeCompleted ||
		childFinished.Run.Metrics.Usage == nil ||
		childFinished.Run.Metrics.Usage.InputTokens != 13 ||
		childFinished.Run.Metrics.Usage.OutputTokens != 5 {
		t.Fatalf("child terminal run = %+v, want child-owned terminal metrics", childFinished)
	}
	if !effects.terminalized(spec.SessionID, "run_child") ||
		!effects.terminalized(spec.SessionID, spec.RunID) {
		t.Fatal("child and root were not independently terminalized")
	}
	commits := effects.commitSnapshot()
	childTerminalAt, rootTerminalAt := -1, -1
	for index, commit := range commits {
		if commit.State != StateTerminalize {
			continue
		}
		switch commit.RunID {
		case "run_child":
			childTerminalAt = index
		case spec.RunID:
			rootTerminalAt = index
		}
	}
	if childTerminalAt < 0 || rootTerminalAt < 0 || childTerminalAt >= rootTerminalAt {
		t.Fatalf(
			"terminal commit order child/root = %d/%d, want child before root",
			childTerminalAt,
			rootTerminalAt,
		)
	}
}

func TestCoordinatorKeepsConcurrentSiblingSegmentsIsolated(t *testing.T) {
	requestA, confirmationA := NewChildOpeningRequest(time.Now())
	requestB, confirmationB := NewChildOpeningRequest(time.Now())
	rootSource := ExecutorSource{ProcessID: "process_root"}
	childA := ExecutorSource{
		ProcessID:   "process_child_a",
		ParentID:    rootSource.ProcessID,
		SpawnCallID: "provider_call_a",
	}
	childB := ExecutorSource{
		ProcessID:   "process_child_b",
		ParentID:    rootSource.ProcessID,
		SpawnCallID: "provider_call_b",
	}
	childUsage := func(model string, prompt int64) *TurnUsage {
		return &TurnUsage{
			Tokens: accounting.TokenUsage{PromptTokens: prompt, CompletionTokens: 1},
			ByModel: []accounting.ModelUsage{{
				Model: model,
				TokenUsage: accounting.TokenUsage{
					PromptTokens:     prompt,
					CompletionTokens: 1,
				},
				Calls: 1,
			}},
			Steps: 1,
		}
	}
	executor := &fakeExecutor{executorEvents: []ExecutorEvent{
		{Source: rootSource, Payload: ToolCallStart{
			CallID: "canonical_a", SourceCallID: childA.SpawnCallID, ToolName: "delegate_task", Arguments: `{}`,
		}},
		{Source: rootSource, Payload: ToolCallStart{
			CallID: "canonical_b", SourceCallID: childB.SpawnCallID, ToolName: "delegate_task", Arguments: `{}`,
		}},
		{Source: childA, Payload: requestA},
		{Source: childB, Payload: requestB},
		{Source: childA, Payload: MessageDelta{Text: "alpha"}},
		{Source: childB, Payload: MessageDelta{Text: "beta"}},
		{Source: childB, Payload: TurnEnd{
			Reason: execution.OutcomeCompleted,
			Usage:  childUsage("model-b", 7),
		}},
		{Source: childA, Payload: TurnEnd{
			Reason: execution.OutcomeCompleted,
			Usage:  childUsage("model-a", 5),
		}},
		{Source: rootSource, Payload: ToolCallEnd{CallID: "canonical_a", OutputText: "alpha"}},
		{Source: rootSource, Payload: ToolCallEnd{CallID: "canonical_b", OutputText: "beta"}},
		{Source: rootSource, Payload: TurnEnd{Reason: execution.OutcomeCompleted}},
	}}
	effects := &fakeEffects{}
	coordinator := testCoordinator(executor, effects)
	runIDs := []string{"run_child_a", "run_child_b"}
	segmentIDs := []string{"seg_child_a", "seg_child_b"}
	coordinator.newRunID = func() string {
		id := runIDs[0]
		runIDs = runIDs[1:]
		return id
	}
	coordinator.newSegmentID = func() string {
		id := segmentIDs[0]
		segmentIDs = segmentIDs[1:]
		return id
	}

	stream, err := coordinator.openSegment(t.Context(), testSegment())
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	events := collectEvents(stream)
	if _, err := confirmationA.Await(t.Context()); err != nil {
		t.Fatalf("child A opening: %v", err)
	}
	if _, err := confirmationB.Await(t.Context()); err != nil {
		t.Fatalf("child B opening: %v", err)
	}

	type childProjection struct {
		text     string
		segment  string
		finished *transcript.Run
	}
	children := map[string]*childProjection{
		"run_child_a": {segment: "seg_child_a"},
		"run_child_b": {segment: "seg_child_b"},
	}
	for _, event := range events {
		child := children[event.RunID]
		if child == nil {
			continue
		}
		if event.SegmentID != child.segment {
			t.Fatalf("run %q event segment = %q, want %q", event.RunID, event.SegmentID, child.segment)
		}
		switch payload := event.Payload.(type) {
		case ItemCompleted:
			if payload.Item.Kind == transcript.AgentMessage {
				child.text = payload.Item.Content[0].Text
			}
		case SegmentFinished:
			run := payload.Run
			child.finished = &run
		}
	}
	for runID, want := range map[string]struct {
		text   string
		prompt int64
		model  string
	}{
		"run_child_a": {text: "alpha", prompt: 5, model: "model-a"},
		"run_child_b": {text: "beta", prompt: 7, model: "model-b"},
	} {
		child := children[runID]
		if child.text != want.text ||
			child.finished == nil ||
			child.finished.Metrics.Steps != 1 ||
			child.finished.Metrics.Usage == nil ||
			child.finished.Metrics.Usage.InputTokens != want.prompt ||
			len(child.finished.Metrics.Usage.ByModel) != 1 {
			t.Fatalf("sibling %q projection = %+v", runID, child)
		}
		if _, ok := child.finished.Metrics.Usage.ByModel[want.model]; !ok {
			t.Fatalf("sibling %q usage = %+v, want model %q", runID, child.finished.Metrics.Usage, want.model)
		}
	}
}

func TestCoordinatorProjectsNestedChildrenWithExactLineageAndPostorderTerminal(t *testing.T) {
	childRequest, childConfirmation := NewChildOpeningRequest(time.Now())
	grandchildRequest, grandchildConfirmation := NewChildOpeningRequest(time.Now())
	rootSource := ExecutorSource{ProcessID: "process_root"}
	childSource := ExecutorSource{
		ProcessID:   "process_child",
		ParentID:    rootSource.ProcessID,
		SpawnCallID: "provider_call_child",
	}
	grandchildSource := ExecutorSource{
		ProcessID:   "process_grandchild",
		ParentID:    childSource.ProcessID,
		SpawnCallID: "provider_call_grandchild",
	}
	usage := func(prompt int64, calls int) *TurnUsage {
		return &TurnUsage{
			Tokens: accounting.TokenUsage{PromptTokens: prompt, CompletionTokens: int64(calls)},
			ByModel: []accounting.ModelUsage{{
				Model: "model",
				TokenUsage: accounting.TokenUsage{
					PromptTokens:     prompt,
					CompletionTokens: int64(calls),
				},
				Calls: calls,
			}},
			Steps: calls,
		}
	}
	executor := &fakeExecutor{executorEvents: []ExecutorEvent{
		{Source: rootSource, Payload: ToolCallStart{
			CallID: "canonical_child", SourceCallID: childSource.SpawnCallID, ToolName: "delegate_task", Arguments: `{}`,
		}},
		{Source: childSource, Payload: childRequest},
		{Source: childSource, Payload: ToolCallStart{
			CallID: "canonical_grandchild", SourceCallID: grandchildSource.SpawnCallID, ToolName: "delegate_task", Arguments: `{}`,
		}},
		{Source: grandchildSource, Payload: grandchildRequest},
		{Source: grandchildSource, Payload: MessageDelta{Text: "leaf"}},
		{Source: grandchildSource, Payload: TurnEnd{
			Reason: execution.OutcomeCompleted,
			Usage:  usage(3, 1),
		}},
		{Source: childSource, Payload: ToolCallEnd{
			CallID: "canonical_grandchild", OutputText: "leaf",
		}},
		{Source: childSource, Payload: MessageDelta{Text: "branch"}},
		{Source: childSource, Payload: TurnEnd{
			Reason: execution.OutcomeCompleted,
			Usage:  usage(9, 3),
		}},
		{Source: rootSource, Payload: ToolCallEnd{
			CallID: "canonical_child", OutputText: "branch",
		}},
		{Source: rootSource, Payload: TurnEnd{Reason: execution.OutcomeCompleted}},
	}}
	effects := &fakeEffects{}
	coordinator := testCoordinator(executor, effects)
	runIDs := []string{"run_child", "run_grandchild"}
	segmentIDs := []string{"seg_child", "seg_grandchild"}
	coordinator.newRunID = func() string {
		id := runIDs[0]
		runIDs = runIDs[1:]
		return id
	}
	coordinator.newSegmentID = func() string {
		id := segmentIDs[0]
		segmentIDs = segmentIDs[1:]
		return id
	}

	stream, err := coordinator.openSegment(t.Context(), testSegment())
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	events := collectEvents(stream)
	if _, err := childConfirmation.Await(t.Context()); err != nil {
		t.Fatalf("child opening: %v", err)
	}
	if _, err := grandchildConfirmation.Await(t.Context()); err != nil {
		t.Fatalf("grandchild opening: %v", err)
	}
	for index, event := range events {
		wantSequence := uint64(index + 1)
		if event.Sequence != wantSequence {
			t.Fatalf("event[%d] sequence = %d, want %d", index, event.Sequence, wantSequence)
		}
		position, err := replaycursor.Decode(event.Cursor)
		if err != nil {
			t.Fatalf("event[%d] cursor: %v", index, err)
		}
		if position.Epoch != coordinator.epoch ||
			position.RunID != testRunID ||
			position.SegmentID != testSegmentID ||
			position.Sequence != wantSequence {
			t.Fatalf(
				"event[%d] cursor = %+v, want root stream %s/%s at %d",
				index,
				position,
				testRunID,
				testSegmentID,
				wantSequence,
			)
		}
	}

	openings := effects.openingSnapshot()
	if len(openings) != 3 || openings[1].Admit == nil || openings[2].Admit == nil {
		t.Fatalf("openings = %+v, want root, child, grandchild", openings)
	}
	child := openings[1].Admit
	grandchild := openings[2].Admit
	if child.ParentRunID != "run_1" ||
		child.RootRunID != "run_1" ||
		grandchild.ParentRunID != child.RunID ||
		grandchild.RootRunID != "run_1" ||
		grandchild.SpawnedByItemID != "item_seg_child_1" {
		t.Fatalf("nested drafts child=%+v grandchild=%+v", child, grandchild)
	}

	var terminalOrder []string
	for _, event := range events {
		if _, ok := event.Payload.(SegmentFinished); ok {
			terminalOrder = append(terminalOrder, event.RunID)
		}
	}
	wantTerminalOrder := []string{"run_grandchild", "run_child", "run_1"}
	if len(terminalOrder) != len(wantTerminalOrder) {
		t.Fatalf("terminal order = %v, want %v", terminalOrder, wantTerminalOrder)
	}
	for index := range wantTerminalOrder {
		if terminalOrder[index] != wantTerminalOrder[index] {
			t.Fatalf("terminal order = %v, want %v", terminalOrder, wantTerminalOrder)
		}
	}
}

func TestCoordinatorDrainedStreamClosesNestedChildrenBeforeAncestors(t *testing.T) {
	childRequest, childConfirmation := NewChildOpeningRequest(time.Now())
	grandchildRequest, grandchildConfirmation := NewChildOpeningRequest(time.Now())
	rootSource := ExecutorSource{ProcessID: "process_root"}
	childSource := ExecutorSource{
		ProcessID: "process_child", ParentID: rootSource.ProcessID, SpawnCallID: "spawn_child",
	}
	grandchildSource := ExecutorSource{
		ProcessID: "process_grandchild", ParentID: childSource.ProcessID, SpawnCallID: "spawn_grandchild",
	}
	executor := &fakeExecutor{executorEvents: []ExecutorEvent{
		{Source: rootSource, Payload: ToolCallStart{
			CallID: "child_call", SourceCallID: childSource.SpawnCallID, ToolName: "delegate_task", Arguments: `{}`,
		}},
		{Source: childSource, Payload: childRequest},
		{Source: childSource, Payload: ToolCallStart{
			CallID: "grandchild_call", SourceCallID: grandchildSource.SpawnCallID, ToolName: "delegate_task", Arguments: `{}`,
		}},
		{Source: grandchildSource, Payload: grandchildRequest},
		// Deliberately drain with all three reducers active.
	}}
	effects := &fakeEffects{}
	coordinator := testCoordinator(executor, effects)
	runIDs := []string{"run_child", "run_grandchild"}
	segmentIDs := []string{"seg_child", "seg_grandchild"}
	coordinator.newRunID = func() string {
		id := runIDs[0]
		runIDs = runIDs[1:]
		return id
	}
	coordinator.newSegmentID = func() string {
		id := segmentIDs[0]
		segmentIDs = segmentIDs[1:]
		return id
	}

	stream, err := coordinator.openSegment(t.Context(), testSegment())
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	events := collectEvents(stream)
	if _, err := childConfirmation.Await(t.Context()); err != nil {
		t.Fatalf("child opening: %v", err)
	}
	if _, err := grandchildConfirmation.Await(t.Context()); err != nil {
		t.Fatalf("grandchild opening: %v", err)
	}

	var terminals []transcript.Run
	for _, event := range events {
		if finished, ok := event.Payload.(SegmentFinished); ok {
			terminals = append(terminals, finished.Run)
		}
	}
	wantOrder := []string{"run_grandchild", "run_child", "run_1"}
	if len(terminals) != len(wantOrder) {
		t.Fatalf("terminals = %+v, want %v", terminals, wantOrder)
	}
	for index, run := range terminals {
		if run.ID != wantOrder[index] ||
			run.Outcome == nil ||
			*run.Outcome != execution.OutcomeCanceled {
			t.Fatalf("terminal[%d] = %+v, want canceled %q", index, run, wantOrder[index])
		}
	}
}

func TestCoordinatorClosesActiveChildrenBeforeRejectingRootTerminal(t *testing.T) {
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
				ToolName:     "delegate_task",
				Arguments:    `{}`,
			},
		},
		{Source: childSource, Payload: request},
		// A correct executor publishes the child's terminal boundary first.
		// This deliberately violates that ordering to prove the application
		// closes the durable tree instead of leaving an active child orphan.
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
	events := collectEvents(stream)
	if _, err := confirmation.Await(t.Context()); err != nil {
		t.Fatalf("child opening confirmation: %v", err)
	}

	var terminalRuns []transcript.Run
	for _, event := range events {
		if finished, ok := event.Payload.(SegmentFinished); ok {
			terminalRuns = append(terminalRuns, finished.Run)
		}
	}
	if len(terminalRuns) != 2 ||
		terminalRuns[0].ID != "run_child" ||
		terminalRuns[1].ID != "run_1" {
		t.Fatalf("terminal event order = %+v, want child then root", terminalRuns)
	}
	for _, run := range terminalRuns {
		if run.Outcome == nil ||
			*run.Outcome != execution.OutcomeError ||
			run.Error == nil ||
			run.Error.Kind != transcript.InternalProblem {
			t.Fatalf("synthesized terminal = %+v, want internal error", run)
		}
	}
	if !effects.terminalized("ses_1", "run_child") ||
		!effects.terminalized("ses_1", "run_1") {
		t.Fatal("root protocol violation left a non-terminal run in the durable tree")
	}
	if executor.cancels() != 1 {
		t.Fatalf("CancelTurn calls = %d, want 1", executor.cancels())
	}
}

func TestCoordinatorRecoversFromChildTerminalCommitFailureBeforeClosingRoot(t *testing.T) {
	commitErr := errors.New("child terminal commit failed")
	request, confirmation := NewChildOpeningRequest(time.Now())
	rootSource := ExecutorSource{ProcessID: "process_root"}
	childSource := ExecutorSource{
		ProcessID:   "process_child",
		ParentID:    rootSource.ProcessID,
		SpawnCallID: "provider_call_delegate",
	}
	executor := &fakeExecutor{executorEvents: []ExecutorEvent{
		{Source: rootSource, Payload: ToolCallStart{
			CallID: "canonical_call_delegate", SourceCallID: childSource.SpawnCallID, ToolName: "delegate_task", Arguments: `{}`,
		}},
		{Source: childSource, Payload: request},
		{Source: childSource, Payload: TurnEnd{Reason: execution.OutcomeCompleted}},
	}}
	// Child segment.started is part of the atomic opening transaction, so the
	// first CommitEvent attempt is the child's requested completed terminal.
	// Fail only that write so cleanup must replace it with an error terminal
	// before it may close the root.
	effects := &fakeEffects{commitErr: commitErr, commitErrAt: 1}
	coordinator := testCoordinator(executor, effects)
	coordinator.newRunID = func() string { return "run_child" }
	coordinator.newSegmentID = func() string { return "seg_child" }

	stream, err := coordinator.openSegment(t.Context(), testSegment())
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	events := collectEvents(stream)
	if _, err := confirmation.Await(t.Context()); err != nil {
		t.Fatalf("child opening: %v", err)
	}

	var terminals []transcript.Run
	for _, event := range events {
		if finished, ok := event.Payload.(SegmentFinished); ok {
			terminals = append(terminals, finished.Run)
		}
	}
	if len(terminals) != 2 ||
		terminals[0].ID != "run_child" ||
		terminals[1].ID != "run_1" {
		t.Fatalf("terminal order = %+v, want child then root", terminals)
	}
	for _, run := range terminals {
		if run.Outcome == nil ||
			*run.Outcome != execution.OutcomeError ||
			run.Error == nil ||
			run.Error.Kind != transcript.InternalProblem {
			t.Fatalf("recovered terminal = %+v, want internal error", run)
		}
	}
	if !effects.terminalized("ses_1", "run_child") ||
		!effects.terminalized("ses_1", "run_1") {
		t.Fatal("terminal commit failure left a durable active run")
	}
	if executor.cancels() != 1 {
		t.Fatalf("CancelTurn calls = %d, want 1", executor.cancels())
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
				ToolName:     "delegate_task",
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
	if _, err := confirmation.Await(t.Context()); !errors.Is(err, commitErr) {
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

func TestCoordinatorClosesAdmittedSiblingWhenNextOpeningRollsBack(t *testing.T) {
	commitErr := errors.New("second child opening failed")
	requestA, confirmationA := NewChildOpeningRequest(time.Now())
	requestB, confirmationB := NewChildOpeningRequest(time.Now())
	rootSource := ExecutorSource{ProcessID: "process_root"}
	childA := ExecutorSource{
		ProcessID: "process_child_a", ParentID: rootSource.ProcessID, SpawnCallID: "spawn_a",
	}
	childB := ExecutorSource{
		ProcessID: "process_child_b", ParentID: rootSource.ProcessID, SpawnCallID: "spawn_b",
	}
	executor := &fakeExecutor{executorEvents: []ExecutorEvent{
		{Source: rootSource, Payload: ToolCallStart{
			CallID: "call_a", SourceCallID: childA.SpawnCallID, ToolName: "delegate_task", Arguments: `{}`,
		}},
		{Source: rootSource, Payload: ToolCallStart{
			CallID: "call_b", SourceCallID: childB.SpawnCallID, ToolName: "delegate_task", Arguments: `{}`,
		}},
		{Source: childA, Payload: requestA},
		{Source: childB, Payload: requestB},
	}}
	effects := &fakeEffects{openingErr: commitErr, openingErrAt: 3}
	coordinator := testCoordinator(executor, effects)
	runIDs := []string{"run_child_a", "run_child_b"}
	segmentIDs := []string{"seg_child_a", "seg_child_b"}
	coordinator.newRunID = func() string {
		id := runIDs[0]
		runIDs = runIDs[1:]
		return id
	}
	coordinator.newSegmentID = func() string {
		id := segmentIDs[0]
		segmentIDs = segmentIDs[1:]
		return id
	}

	stream, err := coordinator.openSegment(t.Context(), testSegment())
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	events := collectEvents(stream)
	if _, err := confirmationA.Await(t.Context()); err != nil {
		t.Fatalf("first child opening: %v", err)
	}
	if _, err := confirmationB.Await(t.Context()); !errors.Is(err, commitErr) {
		t.Fatalf("second child opening = %v, want %v", err, commitErr)
	}
	openings := effects.openingSnapshot()
	if len(openings) != 2 ||
		openings[1].Admit == nil ||
		openings[1].Admit.RunID != "run_child_a" {
		t.Fatalf("committed openings = %+v, want root and first child only", openings)
	}

	var terminalOrder []string
	for _, event := range events {
		if _, ok := event.Payload.(SegmentFinished); ok {
			terminalOrder = append(terminalOrder, event.RunID)
		}
	}
	wantOrder := []string{"run_child_a", "run_1"}
	if len(terminalOrder) != len(wantOrder) ||
		terminalOrder[0] != wantOrder[0] ||
		terminalOrder[1] != wantOrder[1] {
		t.Fatalf("terminal order = %v, want %v", terminalOrder, wantOrder)
	}
	if effects.terminalized("ses_1", "run_child_b") {
		t.Fatal("rolled-back second child acquired a durable terminal")
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

func TestCoordinatorCommitsCompleteTreeBarrierInDeterministicPostorder(t *testing.T) {
	startedAt := testSegment().CreatedAt
	requestA, confirmationA := NewChildOpeningRequest(startedAt)
	requestB, confirmationB := NewChildOpeningRequest(startedAt)
	requestGrandchild, confirmationGrandchild := NewChildOpeningRequest(startedAt)
	root := ExecutorSource{ProcessID: "process_root"}
	childA := ExecutorSource{
		ProcessID: "process_child_a", ParentID: root.ProcessID, SpawnCallID: "spawn_a",
	}
	childB := ExecutorSource{
		ProcessID: "process_child_b", ParentID: root.ProcessID, SpawnCallID: "spawn_b",
	}
	grandchild := ExecutorSource{
		ProcessID: "process_grandchild", ParentID: childA.ProcessID, SpawnCallID: "spawn_grandchild",
	}
	executor := &fakeExecutor{executorEvents: []ExecutorEvent{
		{Source: root, Payload: ToolCallStart{
			CallID: "call_a", SourceCallID: childA.SpawnCallID, ToolName: "delegate_task", Arguments: `{}`,
		}},
		{Source: root, Payload: ToolCallStart{
			CallID: "call_b", SourceCallID: childB.SpawnCallID, ToolName: "delegate_task", Arguments: `{}`,
		}},
		{Source: childA, Payload: requestA},
		{Source: childB, Payload: requestB},
		{Source: childA, Payload: ToolCallStart{
			CallID: "call_grandchild", SourceCallID: grandchild.SpawnCallID, ToolName: "delegate_task", Arguments: `{}`,
		}},
		{Source: grandchild, Payload: requestGrandchild},
		// Deliberately report sibling B before the deeper descendant. Durable
		// and public ordering follows Run-tree postorder, not executor arrival.
		{Source: root, Payload: TreeInterrupted{Checkpoint: testExecutorCheckpoint(), Suspensions: []ProcessSuspension{
			{
				ProcessID: childB.ProcessID, SuspensionID: "suspension_b",
				Interrupt: treeBarrierQuestion("Continue sibling B?"),
			},
			{
				ProcessID: grandchild.ProcessID, SuspensionID: "suspension_grandchild",
				Interrupt: treeBarrierQuestion("Continue grandchild?"),
			},
		}}},
	}}
	effects := &fakeEffects{}
	coordinator := testCoordinator(executor, effects)
	runIDs := []string{"run_child_a", "run_child_b", "run_grandchild"}
	segmentIDs := []string{"seg_child_a", "seg_child_b", "seg_grandchild"}
	coordinator.newRunID = func() string {
		id := runIDs[0]
		runIDs = runIDs[1:]
		return id
	}
	coordinator.newSegmentID = func() string {
		id := segmentIDs[0]
		segmentIDs = segmentIDs[1:]
		return id
	}

	spec := testSegment()
	spec.ProtocolProfile = execution.RunProtocolProfile{
		ChildRuns:      true,
		InterruptKinds: []execution.InterruptKind{execution.QuestionInterrupt},
	}
	stream, err := coordinator.openSegment(t.Context(), spec)
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	events := collectEvents(stream)
	for name, confirmation := range map[string]ChildOpeningConfirmation{
		"child A": confirmationA, "child B": confirmationB, "grandchild": confirmationGrandchild,
	} {
		if _, err := confirmation.Await(t.Context()); err != nil {
			t.Fatalf("%s opening: %v", name, err)
		}
	}

	barriers := effects.barrierSnapshot()
	if len(barriers) != 1 {
		t.Fatalf("tree barrier commits = %d, want exactly one", len(barriers))
	}
	barrier := barriers[0]
	wantOrder := []string{"run_grandchild", "run_child_a", "run_child_b", "run_1"}
	if len(barrier.Runs) != len(wantOrder) ||
		len(barrier.Pending.Continuations) != len(wantOrder) {
		t.Fatalf(
			"barrier runs/continuations = %d/%d, want %d/%d",
			len(barrier.Runs),
			len(barrier.Pending.Continuations),
			len(wantOrder),
			len(wantOrder),
		)
	}
	for index, wantRunID := range wantOrder {
		commit := barrier.Runs[index]
		continuation := barrier.Pending.Continuations[index]
		if commit.RunID != wantRunID || continuation.RunID != wantRunID {
			t.Fatalf(
				"barrier order[%d] = commit %q continuation %q, want %q",
				index,
				commit.RunID,
				continuation.RunID,
				wantRunID,
			)
		}
		if commit.State != StateSuspend || commit.Run == nil ||
			commit.Run.State != execution.Interrupted {
			t.Fatalf("barrier Run[%d] = %+v, want interrupted suspend", index, commit)
		}
		wantDirect := 0
		if wantRunID == "run_grandchild" || wantRunID == "run_child_b" {
			wantDirect = 1
		}
		if len(commit.Run.Interrupts) != wantDirect {
			t.Fatalf(
				"barrier Run %q direct interrupts = %d, want %d",
				wantRunID,
				len(commit.Run.Interrupts),
				wantDirect,
			)
		}
	}
	if got := []string{
		barrier.Pending.Interrupts[0].RunID,
		barrier.Pending.Interrupts[1].RunID,
	}; got[0] != "run_grandchild" || got[1] != "run_child_b" {
		t.Fatalf("pending interrupt order = %v, want grandchild then child B", got)
	}
	if len(barrier.Pending.Suspensions) != 2 ||
		barrier.Pending.Suspensions[0].ProcessID != grandchild.ProcessID ||
		barrier.Pending.Suspensions[1].ProcessID != childB.ProcessID {
		t.Fatalf("pending suspension bindings = %+v", barrier.Pending.Suspensions)
	}

	var finishedOrder []string
	for _, event := range events {
		if finished, ok := event.Payload.(SegmentFinished); ok &&
			finished.Run.State == execution.Interrupted {
			finishedOrder = append(finishedOrder, finished.Run.ID)
		}
	}
	if len(finishedOrder) != len(wantOrder) {
		t.Fatalf("published interrupted order = %v, want %v", finishedOrder, wantOrder)
	}
	for index := range wantOrder {
		if finishedOrder[index] != wantOrder[index] {
			t.Fatalf("published interrupted order = %v, want %v", finishedOrder, wantOrder)
		}
	}
	if executor.cancels() != 0 {
		t.Fatalf("parked tree canceled executor %d times, want 0", executor.cancels())
	}
}

func TestCoordinatorTreeBarrierCommitFailurePublishesNoInterruptedFact(t *testing.T) {
	root := ExecutorSource{ProcessID: "process_root"}
	executor := &fakeExecutor{executorEvents: []ExecutorEvent{{
		Source: root,
		Payload: TreeInterrupted{Checkpoint: testExecutorCheckpoint(), Suspensions: []ProcessSuspension{{
			ProcessID: root.ProcessID, SuspensionID: "suspension_root",
			Interrupt: treeBarrierQuestion("Continue root?"),
		}}},
	}}}
	effects := &fakeEffects{
		commitErr:   errors.New("tree barrier store unavailable"),
		commitErrAt: 1,
	}
	coordinator := testCoordinator(executor, effects)

	stream, err := coordinator.openSegment(t.Context(), testSegment())
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	events := collectEvents(stream)
	if barriers := effects.barrierSnapshot(); len(barriers) != 0 {
		t.Fatalf("failed tree barrier became durable: %+v", barriers)
	}
	for _, event := range events {
		switch payload := event.Payload.(type) {
		case ItemStarted:
			if payload.Item.Kind == transcript.QuestionItem {
				t.Fatalf("uncommitted question was published: %+v", payload.Item)
			}
		case SegmentFinished:
			if payload.Run.State == execution.Interrupted {
				t.Fatalf("uncommitted interrupted Run was published: %+v", payload.Run)
			}
		}
	}
	if executor.cancels() != 1 {
		t.Fatalf("tree barrier failure canceled executor %d times, want 1", executor.cancels())
	}
}

func TestCoordinatorCommitsSyntheticTerminalBeforeCancelTurn(t *testing.T) {
	executor := &fakeExecutor{
		events:        []ExecutorPayload{TurnInterrupted{Interrupts: []Interrupt{{Kind: execution.InterruptKind(9)}}}},
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
	executor := &fakeExecutor{events: []ExecutorPayload{CompactBoundary{MessagesBefore: 4, MessagesAfter: 2}}}
	effects := &fakeEffects{commitErr: errors.New("store down")}
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
	executor := &fakeExecutor{startErr: errors.New("boom")}
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
		"run_1": runForSegment(testSegment()),
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
