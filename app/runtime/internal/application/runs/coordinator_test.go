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
	"github.com/Tangerg/lynx/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	runfixture "github.com/Tangerg/lynx/app/runtime/internal/testsupport/runfixture"
	corechat "github.com/Tangerg/lynx/core/chat"
)

func mustTreeContinuation(t *testing.T, pending Pending) *treeContinuation {
	t.Helper()
	continuation, err := treeContinuationFromPending(pending)
	if err != nil {
		t.Fatalf("tree continuation: %v", err)
	}
	return continuation
}

func testTreeContinuation(pending Pending) *treeContinuation {
	return &treeContinuation{
		rootRunID:     pending.RootRunID,
		sessionID:     pending.SessionID,
		executorID:    pending.ExecutorID,
		goalLeaseID:   pending.GoalLeaseID,
		interrupts:    slices.Clone(pending.Interrupts),
		continuations: slices.Clone(pending.Continuations),
		capabilities:  pending.Capabilities,
	}
}

// These fakes exercise the application-owned reducer and journal. Delivery
// protocol values deliberately do not appear here.
type fakeExecutor struct {
	events         []ExecutorPayload
	executorEvents []ExecutorEvent
	block          bool
	mu             sync.Mutex
	released       int
	startErr       error
	releaseErr     error
	releaseStarted chan struct{}
	allowRelease   chan struct{}
	cancelSignal   chan struct{}
	cancelOnce     sync.Once
}

// childStartFixture is test scenario syntax. fake executors translate it
// into the production reservation → conclusive-start protocol before the Run
// pump sees any event.
type childStartFixture struct {
	executorPayloadBase
	StartedAt time.Time
	result    chan childStartFixtureResult
}

type childStartReceipt struct {
	result <-chan childStartFixtureResult
}

type childStartFixtureResult struct {
	binding ChildRunBinding
	err     error
}

func newChildStartFixture(startedAt time.Time) (childStartFixture, childStartReceipt) {
	result := make(chan childStartFixtureResult, 1)
	return childStartFixture{StartedAt: startedAt, result: result}, childStartReceipt{result: result}
}

func (fixture childStartFixture) complete(binding ChildRunBinding, err error) {
	fixture.result <- childStartFixtureResult{binding: binding, err: err}
}

func (receipt childStartReceipt) Await(ctx context.Context) (ChildRunBinding, error) {
	select {
	case result := <-receipt.result:
		return result.binding, result.err
	case <-ctx.Done():
		return ChildRunBinding{}, ctx.Err()
	}
}

func yieldChildStart(
	ctx context.Context,
	yield func(ExecutorEvent) bool,
	member ExecutorMember,
	fixture childStartFixture,
) bool {
	reservation, reservationReceipt := NewChildRunReservationRequest(fixture.StartedAt)
	if !yield(ExecutorEvent{Member: member, Payload: reservation}) {
		fixture.complete(ChildRunBinding{}, ctx.Err())
		return false
	}
	binding, err := reservationReceipt.Await(ctx)
	if err == nil {
		outcome, outcomeReceipt := NewChildRunStartOutcomeRequest(binding, ChildRunStarted)
		if !yield(ExecutorEvent{Member: member, Payload: outcome}) {
			fixture.complete(ChildRunBinding{}, ctx.Err())
			return false
		}
		err = outcomeReceipt.Await(ctx)
	}
	fixture.complete(binding, err)
	if err == nil {
		return true
	}
	failure := run.Failure{
		Kind:   run.FailureInternal,
		Detail: "child Run start failed",
	}
	yield(ExecutorEvent{
		Member:  ExecutorMember{MemberID: member.ParentID},
		Payload: SegmentEnded{Reason: run.OutcomeFailed, Failure: &failure},
	})
	return false
}

type acknowledgedChildExecutor struct {
	rootMember   ExecutorMember
	childMember  ExecutorMember
	request      childStartFixture
	confirmation childStartReceipt
	childStarted chan struct{}
}

type acknowledgedNativeChildExecutor struct {
	rootMember         ExecutorMember
	childMember        ExecutorMember
	reservation        ChildRunReservationRequest
	reservationReceipt ChildRunReservationReceipt
}

func (e *acknowledgedNativeChildExecutor) Observe(
	ctx context.Context,
	_ ExecutorRef,
) (iter.Seq[ExecutorEvent], error) {
	return func(yield func(ExecutorEvent) bool) {
		if !yield(ExecutorEvent{
			Member: e.rootMember,
			Payload: ToolCallStarted{
				CallID: "delegate-call", ModelCallSequence: 1,
				SourceCallID: e.childMember.SpawnCallID, ToolCallIndex: 0,
				ToolName: "delegate_task", Arguments: `{"summary":"child"}`,
				SafetyClass: tool.SafetyClassExec,
			},
		}) || !yield(ExecutorEvent{Member: e.childMember, Payload: e.reservation}) {
			return
		}
		binding, err := e.reservationReceipt.Await(ctx)
		if err != nil {
			return
		}
		outcome, receipt := NewChildRunStartOutcomeRequest(binding, ChildRunStarted)
		if !yield(ExecutorEvent{Member: e.childMember, Payload: outcome}) || receipt.Await(ctx) != nil {
			return
		}
		// A fresh request carrying the same conclusive result proves pump-level
		// idempotence rather than merely re-reading one receipt.
		replay, replayReceipt := NewChildRunStartOutcomeRequest(binding, ChildRunStarted)
		if !yield(ExecutorEvent{Member: e.childMember, Payload: replay}) || replayReceipt.Await(ctx) != nil {
			return
		}
		if !yield(ExecutorEvent{
			Member: e.childMember, Payload: SegmentEnded{Reason: run.OutcomeCompleted},
		}) {
			return
		}
		result := tool.StringResult(`{"reply":"done"}`)
		if !yield(ExecutorEvent{
			Member: e.rootMember,
			Payload: ToolCallFinished{
				CallID: "delegate-call", Arguments: `{"summary":"child"}`, Result: &result,
			},
		}) {
			return
		}
		yield(ExecutorEvent{Member: e.rootMember, Payload: SegmentEnded{Reason: run.OutcomeCompleted}})
	}, nil
}

func (*acknowledgedNativeChildExecutor) Release(context.Context, ExecutorRef) error { return nil }

type cancellableChildExecutor struct {
	rootMember      ExecutorMember
	childMember     ExecutorMember
	request         childStartFixture
	confirmation    childStartReceipt
	childOpened     chan struct{}
	cancelRequested chan struct{}
	finishRoot      chan struct{}
}

func (e *cancellableChildExecutor) Observe(
	ctx context.Context,
	_ ExecutorRef,
) (iter.Seq[ExecutorEvent], error) {
	return func(yield func(ExecutorEvent) bool) {
		if !yield(ExecutorEvent{
			Member: e.rootMember,
			Payload: ToolCallStarted{
				CallID:       "canonical_child",
				SourceCallID: e.childMember.SpawnCallID,
				ToolName:     "delegate_task",
				Arguments:    `{}`,
			},
		}) {
			return
		}
		if !yieldChildStart(ctx, yield, e.childMember, e.request) {
			return
		}
		close(e.childOpened)
		select {
		case <-e.cancelRequested:
		case <-ctx.Done():
			return
		}
		if !yield(ExecutorEvent{
			Member:  e.childMember,
			Payload: SegmentEnded{Reason: run.OutcomeCanceled},
		}) {
			return
		}
		if !yield(ExecutorEvent{
			Member: e.rootMember,
			Payload: ToolCallFinished{
				CallID: "canonical_child",
				Failure: &tool.Failure{
					Kind:   tool.FailureExecution,
					Detail: "executor was killed",
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
			Member:  e.rootMember,
			Payload: SegmentEnded{Reason: run.OutcomeCompleted},
		})
	}, nil
}

func (*cancellableChildExecutor) Release(context.Context, ExecutorRef) error {
	return nil
}

func (e *acknowledgedChildExecutor) Observe(ctx context.Context, _ ExecutorRef) (iter.Seq[ExecutorEvent], error) {
	return func(yield func(ExecutorEvent) bool) {
		if !yield(ExecutorEvent{
			Member: e.rootMember,
			Payload: ToolCallStarted{
				CallID:       "canonical_call_delegate",
				SourceCallID: e.childMember.SpawnCallID,
				ToolName:     "delegate_task",
				Arguments:    `{}`,
			},
		}) {
			return
		}
		if !yieldChildStart(ctx, yield, e.childMember, e.request) {
			return
		}
		close(e.childStarted)
		if !yield(ExecutorEvent{
			Member:  e.childMember,
			Payload: SegmentEnded{Reason: run.OutcomeCompleted},
		}) {
			return
		}
		yield(ExecutorEvent{
			Member:  e.rootMember,
			Payload: SegmentEnded{Reason: run.OutcomeCompleted},
		})
	}, nil
}

func (*acknowledgedChildExecutor) Release(context.Context, ExecutorRef) error {
	return nil
}

func (f *fakeExecutor) Observe(ctx context.Context, _ ExecutorRef) (iter.Seq[ExecutorEvent], error) {
	if f.startErr != nil {
		return nil, f.startErr
	}
	return func(yield func(ExecutorEvent) bool) {
		if f.block {
			select {
			case <-f.rootCancellationSignal():
				yield(ExecutorEvent{
					Member:  ExecutorMember{MemberID: "member_root"},
					Payload: SegmentEnded{Reason: run.OutcomeCanceled},
				})
			case <-ctx.Done():
			}
			return
		}
		events := f.executorEvents
		if events == nil {
			events = make([]ExecutorEvent, len(f.events))
			for index, event := range f.events {
				events[index] = ExecutorEvent{
					Member:  ExecutorMember{MemberID: "member_root"},
					Payload: event,
				}
			}
		}
		for _, event := range events {
			if fixture, ok := event.Payload.(childStartFixture); ok {
				if !yieldChildStart(ctx, yield, event.Member, fixture) {
					return
				}
				continue
			}
			if ctx.Err() != nil || !yield(event) {
				return
			}
		}
	}, nil
}

func (f *fakeExecutor) rootCancellationSignal() chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.cancelSignal == nil {
		f.cancelSignal = make(chan struct{})
	}
	return f.cancelSignal
}

func (f *fakeExecutor) requestRootCancellation() {
	signal := f.rootCancellationSignal()
	f.cancelOnce.Do(func() { close(signal) })
}

func (f *fakeExecutor) RequestRootCancellation(context.Context, ExecutorRef, string) error {
	f.requestRootCancellation()
	return nil
}

func (f *fakeExecutor) Release(context.Context, ExecutorRef) error {
	if f.releaseStarted != nil {
		close(f.releaseStarted)
	}
	if f.allowRelease != nil {
		<-f.allowRelease
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.released++
	return f.releaseErr
}

func (f *fakeExecutor) releases() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.released
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
	commitErrCount  int
	commitAttempts  int
	rejectCanceled  bool
	suspendStarted  chan<- struct{}
	suspendCanceled chan<- struct{}
	suspendRelease  <-chan struct{}
	terminalStarted chan<- struct{}
	terminalRelease <-chan struct{}
	finishStarted   chan<- struct{}
	finishRelease   <-chan struct{}
	mutateClaim     func(*ClaimedResume)
	childStarts     map[string]ChildRunStartReservation
	childOutcomes   map[string]ChildRunStartOutcome
}

func (e *fakeEffects) ReserveChildRunStart(
	_ context.Context,
	reservation ChildRunStartReservation,
) error {
	if err := reservation.Validate(); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.childStarts == nil {
		e.childStarts = make(map[string]ChildRunStartReservation)
		e.childOutcomes = make(map[string]ChildRunStartOutcome)
	}
	memberID := reservation.Member.MemberID
	if existing, found := e.childStarts[memberID]; found {
		if existing != reservation {
			return errors.New("fake child start reservation conflict")
		}
		return nil
	}
	e.childStarts[memberID] = reservation
	return nil
}

func (e *fakeEffects) CommitStartedChildRun(
	_ context.Context,
	reservation ChildRunStartReservation,
	opening OpeningCommit,
) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	attempt := len(e.openings) + 1
	if e.openingErr != nil && (e.openingErrAt == 0 || e.openingErrAt == attempt) {
		return e.openingErr
	}
	memberID := reservation.Member.MemberID
	if e.childStarts[memberID] != reservation {
		return errors.New("fake started child has no reservation")
	}
	if outcome := e.childOutcomes[memberID]; outcome.valid() {
		if outcome != ChildRunStarted {
			return errors.New("fake child start outcome conflict")
		}
		return nil
	}
	e.childOutcomes[memberID] = ChildRunStarted
	e.openings = append(e.openings, opening)
	e.commits = append(e.commits, opening.Events...)
	return nil
}

func (e *fakeEffects) AbortChildRunStart(
	_ context.Context,
	reservation ChildRunStartReservation,
) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	memberID := reservation.Member.MemberID
	if e.childStarts[memberID] != reservation {
		return errors.New("fake aborted child has no reservation")
	}
	if outcome := e.childOutcomes[memberID]; outcome.valid() {
		if outcome != ChildRunStartAborted {
			return errors.New("fake child start outcome conflict")
		}
		return nil
	}
	e.childOutcomes[memberID] = ChildRunStartAborted
	return nil
}

func (e *fakeEffects) ClaimResume(_ context.Context, claim ResumeClaimCommit) (ClaimedResume, error) {
	if err := claim.Validate(); err != nil {
		return ClaimedResume{}, err
	}
	checkpoint := testExecutorCheckpoint()
	root, _ := claim.Expected.RootContinuation()
	checkpoint.RootMemberID = root.MemberID
	checkpoint.Scope.SessionID = claim.Expected.SessionID
	checkpoint.Scope.CWD = "/work"
	checkpoint.Scope.WorkspaceCWD = "/work"
	checkpoint.Scope.GoalLeaseID = claim.Expected.GoalLeaseID
	checkpoint.ModelSelection = root.ModelSelection
	checkpoint.Limits = root.Limits
	claimed := ClaimedResume{
		Pending: claim.Expected, Answers: append([]InterruptAnswer(nil), claim.Answers...),
		Checkpoint: checkpoint,
	}
	if e.mutateClaim != nil {
		e.mutateClaim(&claimed)
	}
	return claimed, nil
}

type completeTestProjectionPorts interface {
	OpeningCommitter
	ChildRunStartCommitter
	ResumeClaimCommitter
	EventCommitter
	TreeBarrierCommitter
	WaitingCheckpointReader
	WaitingSubtreeCancellationCommitter
	WorkspaceChangeNotifier
	SegmentFinalizer
}

func testProjectionPorts(ports completeTestProjectionPorts) ProjectionPorts {
	return ProjectionPorts{
		Openings:                    ports,
		ChildStarts:                 ports,
		ResumeClaims:                ports,
		Events:                      ports,
		Barriers:                    ports,
		Checkpoints:                 ports,
		WaitingSubtreeCancellations: ports,
		Workspace:                   ports,
		Finalizer:                   ports,
	}
}

func (e *fakeEffects) ReadWaitingCheckpoint(
	_ context.Context,
	rootMemberID string,
) (ExecutorCheckpoint, error) {
	checkpoint := testExecutorCheckpoint()
	checkpoint.RootMemberID = rootMemberID
	checkpoint.Scope.CWD = "/work"
	checkpoint.Scope.WorkspaceCWD = "/work"
	return checkpoint, nil
}

type blockingChildOpeningEffects struct {
	*fakeEffects
	started chan<- struct{}
	release <-chan struct{}
}

func (effects *blockingChildOpeningEffects) CommitStartedChildRun(
	ctx context.Context,
	reservation ChildRunStartReservation,
	opening OpeningCommit,
) error {
	effects.started <- struct{}{}
	select {
	case <-effects.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	return effects.fakeEffects.CommitStartedChildRun(ctx, reservation, opening)
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
	if e.shouldFailCommitAttempt() {
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
	if e.shouldFailCommitAttempt() {
		return e.commitErr
	}
	e.barriers = append(e.barriers, barrier)
	e.commits = append(e.commits, barrier.Runs...)
	return nil
}

func (e *fakeEffects) shouldFailCommitAttempt() bool {
	if e.commitErr == nil {
		return false
	}
	if e.commitErrAt == 0 {
		return true
	}
	count := max(e.commitErrCount, 1)
	return e.commitAttempts >= e.commitErrAt && e.commitAttempts < e.commitErrAt+count
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

func testCoordinator(executor interface {
	ExecutionObserver
	ExecutionReleaser
}, effects completeTestProjectionPorts) *Coordinator {
	return NewCoordinator(Dependencies{
		Observations: executor,
		Releases:     executor,
		Projection:   testProjectionPorts(effects),
		Admissions:   new(admission.Gate),
		Now: func() time.Time {
			return time.Date(2026, 7, 13, 1, 2, 3, 0, time.UTC)
		},
	})
}

type emptyConversationReader struct{}

func (emptyConversationReader) Read(context.Context, string) ([]corechat.Message, error) {
	return nil, nil
}

func testSegment() segmentSpec {
	return segmentSpec{
		RunID: "run_1", SegmentID: "seg_1", SessionID: "ses_1",
		ExecutorID: "turn_1", ModelSelection: mustSelection("openai", "model"),
		CreatedAt: time.Date(2026, 7, 13, 1, 2, 3, 0, time.UTC),
	}
}

func runForSegment(spec segmentSpec) run.Run {
	return runfixture.MustRestore(run.Snapshot{ID: spec.RunID, SessionID: spec.SessionID, State: run.Running,
		ActiveSegmentID: spec.SegmentID, ModelSelection: spec.ModelSelection,
		GoalLeaseID: spec.GoalLeaseID, Limits: spec.Limits,
		Capabilities: spec.Capabilities,
		CreatedAt:    spec.CreatedAt, UpdatedAt: spec.CreatedAt,
		MessageMark: run.UnknownMessageMark})

}

func TestResumedExecutorRouteRetainsGoalLeaseForTerminalAccounting(t *testing.T) {
	createdAt := time.Date(2026, 7, 30, 1, 2, 3, 0, time.UTC)
	pending := testApprovalPending("member_root", createdAt)
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
	if child == nil || child.memberBound || child.member.MemberID != "member_a" {
		t.Fatalf("restored child route = %+v, want opaque executor member binding without persisted topology", child)
	}

	member := ExecutorMember{MemberID: "member_a", ParentID: "member_root", SpawnCallID: "spawn_a"}
	resolved, err := routes.resolve(member)
	if err != nil || resolved != child || !child.memberBound || child.member != member {
		t.Fatalf("resolve live child member = (%+v, %v), route=%+v", resolved, err, child)
	}
	if _, err := routes.resolve(ExecutorMember{
		MemberID: "member_a", ParentID: "member_root", SpawnCallID: "changed",
	}); err == nil || !strings.Contains(err.Error(), "changed immutable lineage") {
		t.Fatalf("changed live child topology error = %v", err)
	}

	if _, err := routes.resolve(ExecutorMember{
		MemberID: "member_b", ParentID: "member_a", SpawnCallID: "spawn_b",
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
	admission, ok := c.admission.AcquireRun(spec.SessionID, spec.CWD)
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
		Kind: interrupt.Question,
		Question: &QuestionPrompt{
			ToolName:  "ask_user",
			Arguments: `{}`,
			Fields:    []QuestionFieldSpec{{Prompt: prompt, Header: "Decision"}},
		},
	}
}

func TestCoordinatorRejectsUncommittedOpening(t *testing.T) {
	executor := &fakeExecutor{}
	effects := &fakeEffects{openingErr: run.ErrSessionBusy}
	coordinator := testCoordinator(executor, effects)

	events, err := coordinator.openSegment(context.Background(), testSegment())
	if !errors.Is(err, run.ErrSessionBusy) {
		t.Fatalf("openSegment error = %v, want ErrSessionBusy", err)
	}
	if _, ok := coordinator.registry.Get("run_1"); events != nil || ok {
		t.Fatal("an uncommitted opening became visible")
	}
	if executor.releases() != 1 {
		t.Fatalf("Release calls = %d, want 1", executor.releases())
	}
	if effects.finishCount() != 0 {
		t.Fatalf("Finish calls = %d, want none without a committed terminal", effects.finishCount())
	}
}

func TestCoordinatorPreservesUnadmittedExecutorReleaseFailure(t *testing.T) {
	cleanupErr := errors.New("executor cleanup failed")
	executor := &fakeExecutor{releaseErr: cleanupErr}
	openingErr := errors.New("opening commit failed")
	coordinator := testCoordinator(executor, &fakeEffects{openingErr: openingErr})

	_, err := coordinator.openSegment(t.Context(), testSegment())
	if !errors.Is(err, openingErr) || !errors.Is(err, cleanupErr) {
		t.Fatalf("openSegment error = %v, want opening and cleanup failures", err)
	}
	if executor.releases() != 1 {
		t.Fatalf("Release calls = %d, want 1", executor.releases())
	}
}

func TestCoordinatorCommitsCanonicalOpeningAndTerminal(t *testing.T) {
	executor := &fakeExecutor{events: []ExecutorPayload{
		MessageDelta{Text: "hello"},
		SegmentEnded{Reason: run.OutcomeCompleted},
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
	if !ok || started.Run.ID() != "run_1" || started.Run.SessionID() != "ses_1" {
		t.Fatalf("first payload = %#v", events[0].Payload)
	}
	finished, ok := events[len(events)-1].Payload.(SegmentFinished)
	if !ok || !runHasOutcome(finished.Run, run.OutcomeCompleted) {
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
	if executor.releases() != 1 {
		t.Fatalf("terminal executor releases = %d, want 1", executor.releases())
	}
	for index := 1; index < len(events); index++ {
		if events[index-1].Sequence >= events[index].Sequence {
			t.Fatalf("stream positions are not monotonic: %d then %d", events[index-1].Sequence, events[index].Sequence)
		}
	}
}

func TestCoordinatorHoldsSessionAdmissionThroughTerminalMaintenance(t *testing.T) {
	executor := &fakeExecutor{events: []ExecutorPayload{SegmentEnded{Reason: run.OutcomeCompleted}}}
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

func TestCoordinatorCommitsExecutorStartFailureInCanonicalOrder(t *testing.T) {
	executor := &fakeExecutor{events: []ExecutorPayload{
		SegmentEnded{Reason: run.OutcomeFailed, Failure: &run.Failure{Kind: run.FailureInternal, Detail: "the run failed due to an internal error"}},
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
	if !runHasOutcome(finished.Run, run.OutcomeFailed) {
		t.Fatalf("outcome = %v, want error", finished.Run.Snapshot().Outcome)
	}
	if !runHasFailureKind(finished.Run, run.FailureInternal) {
		t.Fatalf("run failure = %+v, want canonical internal problem", finished.Run.Snapshot().Failure)
	}
	if events[0].Sequence >= events[1].Sequence {
		t.Fatalf("event order = %d then %d, want monotonic", events[0].Sequence, events[1].Sequence)
	}
	if !effects.terminalized("ses_1", "run_1") {
		t.Fatal("executor start failure did not atomically terminalize the run")
	}
}

func TestCoordinatorResumeCommitsBeforeActivation(t *testing.T) {
	executor := &fakeExecutor{}
	effects := &fakeEffects{}
	coordinator := testCoordinator(executor, effects)
	spec := testSegment()
	spec.SegmentID = "seg_2"
	pending := testApprovalPending("member_root", spec.CreatedAt)
	spec.Continuation = mustTreeContinuation(t, pending)
	activatedAfterOpening := false
	spec.BeginExecution = func(context.Context) error {
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
	rootMember := ExecutorMember{MemberID: "member_root"}
	childASource := ExecutorMember{
		MemberID: "member_a", ParentID: "member_root", SpawnCallID: "spawn_a",
	}
	grandchildSource := ExecutorMember{
		MemberID: "member_grandchild", ParentID: "member_a", SpawnCallID: "spawn_grandchild",
	}
	childBSource := ExecutorMember{
		MemberID: "member_b", ParentID: "member_root", SpawnCallID: "spawn_b",
	}
	executor := &fakeExecutor{executorEvents: []ExecutorEvent{
		{Member: grandchildSource, Payload: SegmentEnded{Reason: run.OutcomeCompleted}},
		{Member: childASource, Payload: SegmentEnded{Reason: run.OutcomeCompleted}},
		{Member: childBSource, Payload: SegmentEnded{Reason: run.OutcomeCompleted}},
		{Member: rootMember, Payload: SegmentEnded{Reason: run.OutcomeCompleted}},
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
	wantRuns := []run.ResumeDraft{
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
			if payload.Run.ActiveSegmentID() != event.SegmentID {
				t.Fatalf(
					"Run %q active segment = %q, envelope = %q",
					event.RunID,
					payload.Run.ActiveSegmentID(),
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
	if !slices.Equal(checkpointDeletes, []string{rootMember.MemberID}) {
		t.Fatalf("terminal executor checkpoint deletes = %v, want root only", checkpointDeletes)
	}
}

func resumedTreePending(createdAt time.Time) Pending {
	question := func(itemID, runID string) transcript.Interrupt {
		return transcript.Interrupt{
			ItemID: itemID, ItemOccurredAt: createdAt,
			RunID: runID, Kind: interrupt.Question,
			Question: &transcript.Question{
				Fields: []transcript.QuestionField{{
					Prompt: "Continue?", Kind: transcript.QuestionText,
				}},
			},
		}
	}
	return Pending{
		RootRunID:  "run_1",
		SessionID:  "ses_1",
		ExecutorID: "turn_1",
		Capabilities: run.Capabilities{
			ChildRuns:      true,
			InterruptKinds: []interrupt.Kind{interrupt.Question},
		},
		Interrupts: []transcript.Interrupt{
			question("item_grandchild", "run_grandchild"),
			question("item_b", "run_b"),
		},
		Bindings: []InterruptBinding{
			{InterruptItemID: "item_grandchild", MemberID: "member_grandchild", RequestID: "request_grandchild"},
			{InterruptItemID: "item_b", MemberID: "member_b", RequestID: "request_b"},
		},
		Continuations: []Continuation{
			{
				RunID:    "run_grandchild",
				MemberID: "member_grandchild",
				Lineage: run.Lineage{
					SpawnedByItemID: "item_spawn_grandchild",
					ParentRunID:     "run_a",
					RootRunID:       "run_1",
				},
				ModelSelection: mustSelection("openai", "model"),
				RunCreatedAt:   createdAt,
			},
			{
				RunID:    "run_a",
				MemberID: "member_a",
				Lineage: run.Lineage{
					SpawnedByItemID: "item_spawn_a",
					ParentRunID:     "run_1",
					RootRunID:       "run_1",
				},
				ModelSelection: mustSelection("openai", "model"),
				RunCreatedAt:   createdAt,
			},
			{
				RunID:    "run_b",
				MemberID: "member_b",
				Lineage: run.Lineage{
					SpawnedByItemID: "item_spawn_b",
					ParentRunID:     "run_1",
					RootRunID:       "run_1",
				},
				ModelSelection: mustSelection("openai", "model"),
				RunCreatedAt:   createdAt,
			},
			{
				RunID:          "run_1",
				MemberID:       "member_root",
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
	spec.BeginExecution = func(context.Context) error { return errors.New("resume failed") }

	stream, err := coordinator.openSegment(context.Background(), spec)
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	events := collectEvents(stream)
	finished, ok := events[len(events)-1].Payload.(SegmentFinished)
	if !ok || !runHasOutcome(finished.Run, run.OutcomeFailed) {
		t.Fatalf("last payload = %#v, want error terminal", events[len(events)-1].Payload)
	}
	if _, failed := finished.Run.Failure(); !failed {
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
	spec.BeginExecution = func(context.Context) error { return errors.New("resume failed") }

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
			if !runHasOutcome(payload.Run, run.OutcomeFailed) {
				t.Fatalf("Run %q outcome = %v, want error", event.RunID, payload.Run.Snapshot().Outcome)
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
	executor := &fakeExecutor{events: []ExecutorPayload{SegmentInterrupted{Interrupts: []Interrupt{{Kind: interrupt.Kind(9)}}}}}
	effects := &fakeEffects{}
	coordinator := testCoordinator(executor, effects)

	stream, err := coordinator.openSegment(t.Context(), testSegment())
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	events := collectEvents(stream)
	finished, ok := events[len(events)-1].Payload.(SegmentFinished)
	if !ok || !runHasOutcome(finished.Run, run.OutcomeFailed) {
		t.Fatalf("last payload = %#v, want error terminal", events[len(events)-1].Payload)
	}
	if executor.releases() != 1 {
		t.Fatalf("Release calls = %d, want 1", executor.releases())
	}
	if !effects.terminalized("ses_1", "run_1") {
		t.Fatal("malformed interrupt did not terminalize the run")
	}
}

func TestCoordinatorProtocolViolationAbortsExecutorAndTerminalizes(t *testing.T) {
	tests := []struct {
		name  string
		event ExecutionFact
	}{
		{name: "unknown event", event: unsupportedEngineEvent{}},
		{name: "invalid terminal outcome", event: SegmentEnded{Reason: run.Outcome(255)}},
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
			if !ok || !runHasOutcome(finished.Run, run.OutcomeFailed) {
				t.Fatalf("last payload = %#v, want error terminal", events[1].Payload)
			}
			if !runHasFailureKind(finished.Run, run.FailureInternal) {
				t.Fatalf("run failure = %+v, want canonical internal problem", finished.Run.Snapshot().Failure)
			}
			if executor.releases() != 1 {
				t.Fatalf("Release calls = %d, want 1", executor.releases())
			}
			if !effects.terminalized("ses_1", "run_1") {
				t.Fatal("executor protocol violation did not terminalize the run")
			}
		})
	}
}

func TestCoordinatorRejectsUnadmittedChildSource(t *testing.T) {
	executor := &fakeExecutor{executorEvents: []ExecutorEvent{{
		Member: ExecutorMember{
			MemberID:    "member_child",
			ParentID:    "member_root",
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
	if !ok || !runHasOutcome(finished.Run, run.OutcomeFailed) {
		t.Fatalf("last payload = %#v, want error terminal", events[1].Payload)
	}
	if executor.releases() != 1 {
		t.Fatalf("Release calls = %d, want 1", executor.releases())
	}
	if !effects.terminalized("ses_1", "run_1") {
		t.Fatal("unadmitted child member did not terminalize the root run")
	}
}

func TestCoordinatorAtomicallyAdmitsChildRunFromSpawningItem(t *testing.T) {
	startedAt := time.Date(2026, 7, 13, 1, 2, 4, 0, time.FixedZone("test", 8*60*60))
	request, confirmation := newChildStartFixture(startedAt)
	rootMember := ExecutorMember{MemberID: "member_root"}
	childMember := ExecutorMember{
		MemberID:    "member_child",
		ParentID:    rootMember.MemberID,
		SpawnCallID: "provider_call_delegate",
	}
	executor := &fakeExecutor{executorEvents: []ExecutorEvent{
		{
			Member: rootMember,
			Payload: ToolCallStarted{
				CallID:       "canonical_call_delegate",
				SourceCallID: childMember.SpawnCallID,
				ToolName:     "delegate_task",
				Arguments:    `{"description":"delegate"}`,
			},
		},
		{Member: childMember, Payload: request},
		{Member: childMember, Payload: SegmentEnded{Reason: run.OutcomeCompleted}},
		{Member: rootMember, Payload: SegmentEnded{Reason: run.OutcomeCompleted}},
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
		MemberID: childMember.MemberID, RunID: "run_child", ParentRunID: testSegment().RunID,
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
		!draft.Capabilities.IsEmpty() ||
		!draft.CreatedAt.Equal(startedAt) {
		t.Fatalf("child admission draft = %+v", draft)
	}
	if len(child.Events) != 1 || len(child.Events[0].Items) != 1 {
		t.Fatalf("child opening events = %+v, want parent spawning item", child.Events)
	}
	parentCommit := child.Events[0]
	spawningItem := parentCommit.Items[0]
	invocation, present := spawningItem.ToolInvocation()
	if parentCommit.RunID != "run_1" ||
		parentCommit.SessionID != "ses_1" ||
		spawningItem.ID() != draft.SpawnedByItemID ||
		spawningItem.RunID() != "run_1" ||
		spawningItem.SessionID() != "ses_1" ||
		spawningItem.Status() != transcript.ItemRunning ||
		!present || invocation.Name != "delegate_task" {
		t.Fatalf("parent spawning-item commit = %+v", parentCommit)
	}
}

func TestCoordinatorPublishesNativeChildOnlyAfterConclusiveStart(t *testing.T) {
	startedAt := time.Date(2026, 7, 13, 1, 2, 4, 0, time.UTC)
	reservation, receipt := NewChildRunReservationRequest(startedAt)
	rootMember := ExecutorMember{MemberID: "member_root"}
	childMember := ExecutorMember{
		MemberID: "member_child", ParentID: rootMember.MemberID, SpawnCallID: "provider_delegate",
	}
	executor := &acknowledgedNativeChildExecutor{
		rootMember: rootMember, childMember: childMember,
		reservation: reservation, reservationReceipt: receipt,
	}
	effects := &fakeEffects{}
	coordinator := testCoordinator(executor, effects)
	coordinator.newRunID = func() string { return "run_child" }
	coordinator.newSegmentID = func() string { return "segment_child" }
	spec := testSegment()
	spec.Capabilities = run.Capabilities{ChildRuns: true}

	stream, err := coordinator.openSegment(t.Context(), spec)
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	events := collectEvents(stream)
	if len(events) < 4 {
		t.Fatalf("published events = %d, want root and child lifecycle", len(events))
	}
	openings := effects.openingSnapshot()
	if len(openings) != 2 || openings[1].Admit == nil {
		t.Fatalf("openings = %#v, want one conclusive child admission", openings)
	}
	child := openings[1].Admit
	if child.RunID != "run_child" || child.SegmentID != "segment_child" ||
		!child.Capabilities.ChildRuns || !child.CreatedAt.Equal(startedAt) {
		t.Fatalf("child draft = %+v", child)
	}
	effects.mu.Lock()
	storedReservation := effects.childStarts[childMember.MemberID]
	storedOutcome := effects.childOutcomes[childMember.MemberID]
	effects.mu.Unlock()
	if storedReservation.Binding.RunID != child.RunID || storedOutcome != ChildRunStarted {
		t.Fatalf("child start ledger = (%+v, %v)", storedReservation, storedOutcome)
	}
}

type publishedChildLifecycle struct {
	started   *SegmentStarted
	completed *ItemCompleted
	finished  *SegmentFinished
}

func requirePublishedChildLifecycle(
	t *testing.T,
	events []Event,
	runID string,
	segmentID string,
) publishedChildLifecycle {
	t.Helper()
	var lifecycle publishedChildLifecycle
	for index := range events {
		event := events[index]
		if event.RunID != runID {
			continue
		}
		if event.SegmentID != segmentID {
			t.Fatalf("Run %q event[%d] segment = %q, want %q", runID, index, event.SegmentID, segmentID)
		}
		switch payload := event.Payload.(type) {
		case SegmentStarted:
			lifecycle.started = &payload
		case ItemCompleted:
			lifecycle.completed = &payload
		case SegmentFinished:
			lifecycle.finished = &payload
		}
	}
	if lifecycle.started == nil || lifecycle.completed == nil || lifecycle.finished == nil {
		t.Fatalf("Run %q lifecycle = %+v, want started, completed item, and finished events", runID, lifecycle)
	}
	return lifecycle
}

func requireIndependentChildLifecycle(
	t *testing.T,
	lifecycle publishedChildLifecycle,
	spec segmentSpec,
	lineage run.Lineage,
	childRunID string,
	childSegmentID string,
) {
	t.Helper()
	started := lifecycle.started
	if started.Run.Lineage() != lineage ||
		started.Run.ActiveSegmentID() != childSegmentID ||
		started.Run.Limits() != spec.Limits ||
		started.Run.Capabilities().String() != spec.Capabilities.String() {
		t.Fatalf("child opening Run = %+v, want independent inherited segment state", started.Run)
	}
	completed := lifecycle.completed
	if completed.Item.RunID() != childRunID ||
		completed.Item.SessionID() != spec.SessionID ||
		completed.Item.Kind() != transcript.AgentMessage {
		t.Fatalf("child completed item = %+v, want child-owned assistant item", completed)
	}
	finished := lifecycle.finished
	usage, reported := finished.Run.Metrics().Usage()
	if finished.Run.Lineage() != lineage || !runHasOutcome(finished.Run, run.OutcomeCompleted) || !reported ||
		usage.Total.InputTokens != 13 || usage.Total.OutputTokens != 5 {
		t.Fatalf("child terminal Run = %+v, want child-owned terminal metrics", finished)
	}
}

func requireTerminalCommitOrder(t *testing.T, commits []EventCommit, earlierRunID, laterRunID string) {
	t.Helper()
	earlierIndex, laterIndex := -1, -1
	for index, commit := range commits {
		if commit.State != StateTerminalize {
			continue
		}
		switch commit.RunID {
		case earlierRunID:
			earlierIndex = index
		case laterRunID:
			laterIndex = index
		}
	}
	if earlierIndex < 0 || laterIndex < 0 || earlierIndex >= laterIndex {
		t.Fatalf(
			"terminal commit order %s/%s = %d/%d, want first Run before second",
			earlierRunID,
			laterRunID,
			earlierIndex,
			laterIndex,
		)
	}
}

func TestCoordinatorPublishesChildSegmentOnItsOwnRunIdentity(t *testing.T) {
	request, confirmation := newChildStartFixture(time.Date(2026, 7, 13, 1, 2, 4, 0, time.UTC))
	rootMember := ExecutorMember{MemberID: "member_root"}
	childMember := ExecutorMember{
		MemberID:    "member_child",
		ParentID:    rootMember.MemberID,
		SpawnCallID: "provider_call_delegate",
	}
	finalUsage := SegmentUsage{
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
			Member: rootMember,
			Payload: ToolCallStarted{
				CallID:       "canonical_call_delegate",
				SourceCallID: childMember.SpawnCallID,
				ToolName:     "delegate_task",
				Arguments:    `{"description":"delegate"}`,
			},
		},
		{Member: childMember, Payload: request},
		{Member: childMember, Payload: MessageDelta{Text: "child reply"}},
		{Member: childMember, Payload: SegmentEnded{
			Reason: run.OutcomeCompleted,
			Usage:  &finalUsage,
		}},
		{Member: rootMember, Payload: ToolCallFinished{
			CallID:     "canonical_call_delegate",
			OutputText: "child reply",
		}},
		{Member: rootMember, Payload: SegmentEnded{Reason: run.OutcomeCompleted}},
	}}
	effects := &fakeEffects{}
	coordinator := testCoordinator(executor, effects)
	coordinator.newRunID = func() string { return "run_child" }
	coordinator.newSegmentID = func() string { return "seg_child" }
	spec := testSegment()
	spec.Limits = run.Limits{MaxSteps: 20, MaxBudgetUSD: 3}
	spec.Capabilities = run.Capabilities{
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

	lineage := run.Lineage{
		SpawnedByItemID: "item_seg_1_1",
		ParentRunID:     "run_1",
		RootRunID:       "run_1",
	}
	lifecycle := requirePublishedChildLifecycle(t, events, "run_child", "seg_child")
	requireIndependentChildLifecycle(t, lifecycle, spec, lineage, "run_child", "seg_child")
	if !effects.terminalized(spec.SessionID, "run_child") ||
		!effects.terminalized(spec.SessionID, spec.RunID) {
		t.Fatal("child and root were not independently terminalized")
	}
	requireTerminalCommitOrder(t, effects.commitSnapshot(), "run_child", spec.RunID)
}

func TestCoordinatorKeepsConcurrentSiblingSegmentsIsolated(t *testing.T) {
	requestA, confirmationA := newChildStartFixture(time.Now())
	requestB, confirmationB := newChildStartFixture(time.Now())
	rootMember := ExecutorMember{MemberID: "member_root"}
	childA := ExecutorMember{
		MemberID:    "member_child_a",
		ParentID:    rootMember.MemberID,
		SpawnCallID: "provider_call_a",
	}
	childB := ExecutorMember{
		MemberID:    "member_child_b",
		ParentID:    rootMember.MemberID,
		SpawnCallID: "provider_call_b",
	}
	childUsage := func(model string, prompt int64) *SegmentUsage {
		return &SegmentUsage{
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
		{Member: rootMember, Payload: ToolCallStarted{
			CallID: "canonical_a", SourceCallID: childA.SpawnCallID, ToolName: "delegate_task", Arguments: `{}`,
		}},
		{Member: rootMember, Payload: ToolCallStarted{
			CallID: "canonical_b", SourceCallID: childB.SpawnCallID, ToolName: "delegate_task", Arguments: `{}`,
		}},
		{Member: childA, Payload: requestA},
		{Member: childB, Payload: requestB},
		{Member: childA, Payload: MessageDelta{Text: "alpha"}},
		{Member: childB, Payload: MessageDelta{Text: "beta"}},
		{Member: childB, Payload: SegmentEnded{
			Reason: run.OutcomeCompleted,
			Usage:  childUsage("model-b", 7),
		}},
		{Member: childA, Payload: SegmentEnded{
			Reason: run.OutcomeCompleted,
			Usage:  childUsage("model-a", 5),
		}},
		{Member: rootMember, Payload: ToolCallFinished{CallID: "canonical_a", OutputText: "alpha"}},
		{Member: rootMember, Payload: ToolCallFinished{CallID: "canonical_b", OutputText: "beta"}},
		{Member: rootMember, Payload: SegmentEnded{Reason: run.OutcomeCompleted}},
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
		finished *run.Run
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
			if payload.Item.Kind() == transcript.AgentMessage {
				child.text = payload.Item.Content()[0].Text
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
		if child.finished == nil {
			t.Fatalf("sibling %q has no terminal Run", runID)
		}
		usage, reported := child.finished.Metrics().Usage()
		if child.text != want.text ||
			child.finished.Metrics().Steps() != 1 ||
			!reported ||
			usage.Total.InputTokens != want.prompt ||
			len(usage.ByModel) != 1 {
			t.Fatalf("sibling %q projection = %+v", runID, child)
		}
		if _, ok := usage.ByModel[want.model]; !ok {
			t.Fatalf("sibling %q usage = %+v, want model %q", runID, usage, want.model)
		}
	}
}

func TestCoordinatorProjectsNestedChildrenWithExactLineageAndPostorderTerminal(t *testing.T) {
	childRequest, childConfirmation := newChildStartFixture(time.Now())
	grandchildRequest, grandchildConfirmation := newChildStartFixture(time.Now())
	rootMember := ExecutorMember{MemberID: "member_root"}
	childMember := ExecutorMember{
		MemberID:    "member_child",
		ParentID:    rootMember.MemberID,
		SpawnCallID: "provider_call_child",
	}
	grandchildSource := ExecutorMember{
		MemberID:    "member_grandchild",
		ParentID:    childMember.MemberID,
		SpawnCallID: "provider_call_grandchild",
	}
	usage := func(prompt int64, calls int) *SegmentUsage {
		return &SegmentUsage{
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
		{Member: rootMember, Payload: ToolCallStarted{
			CallID: "canonical_child", SourceCallID: childMember.SpawnCallID, ToolName: "delegate_task", Arguments: `{}`,
		}},
		{Member: childMember, Payload: childRequest},
		{Member: childMember, Payload: ToolCallStarted{
			CallID: "canonical_grandchild", SourceCallID: grandchildSource.SpawnCallID, ToolName: "delegate_task", Arguments: `{}`,
		}},
		{Member: grandchildSource, Payload: grandchildRequest},
		{Member: grandchildSource, Payload: MessageDelta{Text: "leaf"}},
		{Member: grandchildSource, Payload: SegmentEnded{
			Reason: run.OutcomeCompleted,
			Usage:  usage(3, 1),
		}},
		{Member: childMember, Payload: ToolCallFinished{
			CallID: "canonical_grandchild", OutputText: "leaf",
		}},
		{Member: childMember, Payload: MessageDelta{Text: "branch"}},
		{Member: childMember, Payload: SegmentEnded{
			Reason: run.OutcomeCompleted,
			Usage:  usage(9, 3),
		}},
		{Member: rootMember, Payload: ToolCallFinished{
			CallID: "canonical_child", OutputText: "branch",
		}},
		{Member: rootMember, Payload: SegmentEnded{Reason: run.OutcomeCompleted}},
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
		position, err := decodeReplayCursor(event.Cursor)
		if err != nil {
			t.Fatalf("event[%d] cursor: %v", index, err)
		}
		if position.epoch != coordinator.epoch ||
			position.runID != testRunID ||
			position.segmentID != testSegmentID ||
			position.sequence != wantSequence {
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
	childRequest, childConfirmation := newChildStartFixture(time.Now())
	grandchildRequest, grandchildConfirmation := newChildStartFixture(time.Now())
	rootMember := ExecutorMember{MemberID: "member_root"}
	childMember := ExecutorMember{
		MemberID: "member_child", ParentID: rootMember.MemberID, SpawnCallID: "spawn_child",
	}
	grandchildSource := ExecutorMember{
		MemberID: "member_grandchild", ParentID: childMember.MemberID, SpawnCallID: "spawn_grandchild",
	}
	executor := &fakeExecutor{executorEvents: []ExecutorEvent{
		{Member: rootMember, Payload: ToolCallStarted{
			CallID: "child_call", SourceCallID: childMember.SpawnCallID, ToolName: "delegate_task", Arguments: `{}`,
		}},
		{Member: childMember, Payload: childRequest},
		{Member: childMember, Payload: ToolCallStarted{
			CallID: "grandchild_call", SourceCallID: grandchildSource.SpawnCallID, ToolName: "delegate_task", Arguments: `{}`,
		}},
		{Member: grandchildSource, Payload: grandchildRequest},
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

	var terminals []run.Run
	for _, event := range events {
		if finished, ok := event.Payload.(SegmentFinished); ok {
			terminals = append(terminals, finished.Run)
		}
	}
	wantOrder := []string{"run_grandchild", "run_child", "run_1"}
	if len(terminals) != len(wantOrder) {
		t.Fatalf("terminals = %+v, want %v", terminals, wantOrder)
	}
	for index, record := range terminals {
		if record.ID() != wantOrder[index] || !runHasOutcome(record, run.OutcomeCanceled) {
			t.Fatalf("terminal[%d] = %+v, want canceled %q", index, record, wantOrder[index])
		}
	}
}

func TestCoordinatorClosesActiveChildrenBeforeRejectingRootTerminal(t *testing.T) {
	request, confirmation := newChildStartFixture(time.Now())
	rootMember := ExecutorMember{MemberID: "member_root"}
	childMember := ExecutorMember{
		MemberID:    "member_child",
		ParentID:    rootMember.MemberID,
		SpawnCallID: "provider_call_delegate",
	}
	executor := &fakeExecutor{executorEvents: []ExecutorEvent{
		{
			Member: rootMember,
			Payload: ToolCallStarted{
				CallID:       "canonical_call_delegate",
				SourceCallID: childMember.SpawnCallID,
				ToolName:     "delegate_task",
				Arguments:    `{}`,
			},
		},
		{Member: childMember, Payload: request},
		// A correct executor publishes the child's terminal boundary first.
		// This deliberately violates that ordering to prove the application
		// closes the durable tree instead of leaving an active child orphan.
		{Member: rootMember, Payload: SegmentEnded{Reason: run.OutcomeCompleted}},
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

	var terminalRuns []run.Run
	for _, event := range events {
		if finished, ok := event.Payload.(SegmentFinished); ok {
			terminalRuns = append(terminalRuns, finished.Run)
		}
	}
	if len(terminalRuns) != 2 ||
		terminalRuns[0].ID() != "run_child" ||
		terminalRuns[1].ID() != "run_1" {
		t.Fatalf("terminal event order = %+v, want child then root", terminalRuns)
	}
	for _, record := range terminalRuns {
		if !runHasOutcome(record, run.OutcomeFailed) || !runHasFailureKind(record, run.FailureInternal) {
			t.Fatalf("synthesized terminal = %+v, want internal error", record)
		}
	}
	if !effects.terminalized("ses_1", "run_child") ||
		!effects.terminalized("ses_1", "run_1") {
		t.Fatal("root protocol violation left a non-terminal run in the durable tree")
	}
	if executor.releases() != 1 {
		t.Fatalf("Release calls = %d, want 1", executor.releases())
	}
}

func TestCoordinatorRecoversFromChildTerminalCommitFailureBeforeClosingRoot(t *testing.T) {
	commitErr := errors.New("child terminal commit failed")
	request, confirmation := newChildStartFixture(time.Now())
	rootMember := ExecutorMember{MemberID: "member_root"}
	childMember := ExecutorMember{
		MemberID:    "member_child",
		ParentID:    rootMember.MemberID,
		SpawnCallID: "provider_call_delegate",
	}
	executor := &fakeExecutor{executorEvents: []ExecutorEvent{
		{Member: rootMember, Payload: ToolCallStarted{
			CallID: "canonical_call_delegate", SourceCallID: childMember.SpawnCallID, ToolName: "delegate_task", Arguments: `{}`,
		}},
		{Member: childMember, Payload: request},
		{Member: childMember, Payload: SegmentEnded{Reason: run.OutcomeCompleted}},
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

	var terminals []run.Run
	for _, event := range events {
		if finished, ok := event.Payload.(SegmentFinished); ok {
			terminals = append(terminals, finished.Run)
		}
	}
	if len(terminals) != 2 ||
		terminals[0].ID() != "run_child" ||
		terminals[1].ID() != "run_1" {
		t.Fatalf("terminal order = %+v, want child then root", terminals)
	}
	for _, record := range terminals {
		if !runHasOutcome(record, run.OutcomeFailed) || !runHasFailureKind(record, run.FailureInternal) {
			t.Fatalf("recovered terminal = %+v, want internal error", record)
		}
	}
	if !effects.terminalized("ses_1", "run_child") ||
		!effects.terminalized("ses_1", "run_1") {
		t.Fatal("terminal commit failure left a durable active run")
	}
	if executor.releases() != 1 {
		t.Fatalf("Release calls = %d, want 1", executor.releases())
	}
}

func runHasFailureKind(record run.Run, expected run.FailureKind) bool {
	failure, failed := record.Failure()
	return failed && failure.Kind == expected
}

func TestCoordinatorRejectsChildWhenAtomicOpeningFails(t *testing.T) {
	commitErr := errors.New("child opening transaction failed")
	request, confirmation := newChildStartFixture(time.Now())
	rootMember := ExecutorMember{MemberID: "member_root"}
	childMember := ExecutorMember{
		MemberID:    "member_child",
		ParentID:    rootMember.MemberID,
		SpawnCallID: "provider_call_delegate",
	}
	executor := &fakeExecutor{executorEvents: []ExecutorEvent{
		{
			Member: rootMember,
			Payload: ToolCallStarted{
				CallID:       "canonical_call_delegate",
				SourceCallID: childMember.SpawnCallID,
				ToolName:     "delegate_task",
				Arguments:    `{}`,
			},
		},
		{Member: childMember, Payload: request},
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
	if !ok || !runHasOutcome(finished.Run, run.OutcomeFailed) {
		t.Fatalf("last payload = %#v, want root error terminal", events[len(events)-1].Payload)
	}
	if !effects.terminalized("ses_1", "run_1") {
		t.Fatal("failed child opening did not terminalize the root")
	}
	if executor.releases() != 1 {
		t.Fatalf("Release calls = %d, want 1", executor.releases())
	}
}

func TestCoordinatorClosesAdmittedSiblingWhenNextOpeningRollsBack(t *testing.T) {
	commitErr := errors.New("second child opening failed")
	requestA, confirmationA := newChildStartFixture(time.Now())
	requestB, confirmationB := newChildStartFixture(time.Now())
	rootMember := ExecutorMember{MemberID: "member_root"}
	childA := ExecutorMember{
		MemberID: "member_child_a", ParentID: rootMember.MemberID, SpawnCallID: "spawn_a",
	}
	childB := ExecutorMember{
		MemberID: "member_child_b", ParentID: rootMember.MemberID, SpawnCallID: "spawn_b",
	}
	executor := &fakeExecutor{executorEvents: []ExecutorEvent{
		{Member: rootMember, Payload: ToolCallStarted{
			CallID: "call_a", SourceCallID: childA.SpawnCallID, ToolName: "delegate_task", Arguments: `{}`,
		}},
		{Member: rootMember, Payload: ToolCallStarted{
			CallID: "call_b", SourceCallID: childB.SpawnCallID, ToolName: "delegate_task", Arguments: `{}`,
		}},
		{Member: childA, Payload: requestA},
		{Member: childB, Payload: requestB},
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
	request, confirmation := newChildStartFixture(time.Now())
	rootMember := ExecutorMember{MemberID: "member_root"}
	childMember := ExecutorMember{
		MemberID:    "member_child",
		ParentID:    rootMember.MemberID,
		SpawnCallID: "provider_call_delegate",
	}
	executor := &acknowledgedChildExecutor{
		rootMember:   rootMember,
		childMember:  childMember,
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

func requireChildOpeningReceipts(
	t *testing.T,
	receipts map[string]childStartReceipt,
) {
	t.Helper()
	for childName, receipt := range receipts {
		if _, err := receipt.Await(t.Context()); err != nil {
			t.Fatalf("%s opening: %v", childName, err)
		}
	}
}

func requireWaitingTreeBarrierPostorder(
	t *testing.T,
	barrier TreeBarrierCommit,
	wantRunIDs []string,
) {
	t.Helper()
	if len(barrier.Runs) != len(wantRunIDs) ||
		len(barrier.Pending.Continuations) != len(wantRunIDs) {
		t.Fatalf(
			"barrier Runs/continuations = %d/%d, want %d/%d",
			len(barrier.Runs),
			len(barrier.Pending.Continuations),
			len(wantRunIDs),
			len(wantRunIDs),
		)
	}
	for index, wantRunID := range wantRunIDs {
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
		if commit.State != StateSuspend || commit.Run == nil || commit.Run.State() != run.Waiting {
			t.Fatalf("barrier Run[%d] = %+v, want waiting suspend", index, commit)
		}
	}
}

func requirePendingInterruptOrder(
	t *testing.T,
	pending Pending,
	wantRunIDs []string,
	wantMemberIDs []string,
) {
	t.Helper()
	if len(pending.Interrupts) != len(wantRunIDs) || len(pending.Bindings) != len(wantMemberIDs) {
		t.Fatalf(
			"pending interrupts/bindings = %d/%d, want %d/%d",
			len(pending.Interrupts),
			len(pending.Bindings),
			len(wantRunIDs),
			len(wantMemberIDs),
		)
	}
	for index := range wantRunIDs {
		if pending.Interrupts[index].RunID != wantRunIDs[index] ||
			pending.Bindings[index].MemberID != wantMemberIDs[index] {
			t.Fatalf(
				"pending order[%d] = Run %q member %q, want Run %q member %q",
				index,
				pending.Interrupts[index].RunID,
				pending.Bindings[index].MemberID,
				wantRunIDs[index],
				wantMemberIDs[index],
			)
		}
	}
}

func requirePublishedWaitingOrder(t *testing.T, events []Event, wantRunIDs []string) {
	t.Helper()
	var actualRunIDs []string
	for _, event := range events {
		finished, ok := event.Payload.(SegmentFinished)
		if ok && finished.Run.State() == run.Waiting {
			actualRunIDs = append(actualRunIDs, finished.Run.ID())
		}
	}
	if !slices.Equal(actualRunIDs, wantRunIDs) {
		t.Fatalf("published waiting order = %v, want %v", actualRunIDs, wantRunIDs)
	}
}

func TestCoordinatorCommitsCompleteTreeBarrierInDeterministicPostorder(t *testing.T) {
	startedAt := testSegment().CreatedAt
	requestA, confirmationA := newChildStartFixture(startedAt)
	requestB, confirmationB := newChildStartFixture(startedAt)
	requestGrandchild, confirmationGrandchild := newChildStartFixture(startedAt)
	root := ExecutorMember{MemberID: "member_root"}
	childA := ExecutorMember{
		MemberID: "member_child_a", ParentID: root.MemberID, SpawnCallID: "spawn_a",
	}
	childB := ExecutorMember{
		MemberID: "member_child_b", ParentID: root.MemberID, SpawnCallID: "spawn_b",
	}
	grandchild := ExecutorMember{
		MemberID: "member_grandchild", ParentID: childA.MemberID, SpawnCallID: "spawn_grandchild",
	}
	executor := &fakeExecutor{executorEvents: []ExecutorEvent{
		{Member: root, Payload: ToolCallStarted{
			CallID: "call_a", SourceCallID: childA.SpawnCallID, ToolName: "delegate_task", Arguments: `{}`,
		}},
		{Member: root, Payload: ToolCallStarted{
			CallID: "call_b", SourceCallID: childB.SpawnCallID, ToolName: "delegate_task", Arguments: `{}`,
		}},
		{Member: childA, Payload: requestA},
		{Member: childB, Payload: requestB},
		{Member: childA, Payload: ToolCallStarted{
			CallID: "call_grandchild", SourceCallID: grandchild.SpawnCallID, ToolName: "delegate_task", Arguments: `{}`,
		}},
		{Member: grandchild, Payload: requestGrandchild},
		// Deliberately report sibling B before the deeper descendant. Durable
		// and public ordering follows Run-tree postorder, not executor arrival.
		{Member: root, Payload: TreeInterrupted{Checkpoint: testExecutorCheckpoint(), Interruptions: []MemberInterruption{
			{
				MemberID: childB.MemberID, RequestID: "request_b",
				Interrupt: treeBarrierQuestion("Continue sibling B?"),
			},
			{
				MemberID: grandchild.MemberID, RequestID: "request_grandchild",
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
	spec.Capabilities = run.Capabilities{
		ChildRuns:      true,
		InterruptKinds: []interrupt.Kind{interrupt.Question},
	}
	stream, err := coordinator.openSegment(t.Context(), spec)
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	events := collectEvents(stream)
	requireChildOpeningReceipts(t, map[string]childStartReceipt{
		"child A": confirmationA, "child B": confirmationB, "grandchild": confirmationGrandchild,
	})

	barriers := effects.barrierSnapshot()
	if len(barriers) != 1 {
		t.Fatalf("tree barrier commits = %d, want exactly one", len(barriers))
	}
	barrier := barriers[0]
	wantOrder := []string{"run_grandchild", "run_child_a", "run_child_b", "run_1"}
	requireWaitingTreeBarrierPostorder(t, barrier, wantOrder)
	requirePendingInterruptOrder(
		t,
		barrier.Pending,
		[]string{"run_grandchild", "run_child_b"},
		[]string{grandchild.MemberID, childB.MemberID},
	)
	requirePublishedWaitingOrder(t, events, wantOrder)
	if executor.releases() != 0 {
		t.Fatalf("parked tree released executor %d times, want 0", executor.releases())
	}
}

func TestCoordinatorTreeBarrierCommitFailurePublishesNoInterruptedFact(t *testing.T) {
	root := ExecutorMember{MemberID: "member_root"}
	executor := &fakeExecutor{executorEvents: []ExecutorEvent{{
		Member: root,
		Payload: TreeInterrupted{Checkpoint: testExecutorCheckpoint(), Interruptions: []MemberInterruption{{
			MemberID: root.MemberID, RequestID: "request_root",
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
			if payload.Run.State() == run.Waiting {
				t.Fatalf("uncommitted waiting Run was published: %+v", payload.Run)
			}
		}
	}
	if executor.releases() != 1 {
		t.Fatalf("tree barrier failure released executor %d times, want 1", executor.releases())
	}
}

func TestCoordinatorCommitsSyntheticTerminalBeforeRelease(t *testing.T) {
	executor := &fakeExecutor{
		events:         []ExecutorPayload{SegmentInterrupted{Interrupts: []Interrupt{{Kind: interrupt.Kind(9)}}}},
		releaseStarted: make(chan struct{}),
		allowRelease:   make(chan struct{}),
	}
	effects := &fakeEffects{}
	coordinator := testCoordinator(executor, effects)
	stream, err := coordinator.openSegment(t.Context(), testSegment())
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}

	select {
	case <-executor.releaseStarted:
	case <-time.After(time.Second):
		t.Fatal("Release did not start")
	}
	if !effects.terminalized("ses_1", "run_1") {
		t.Fatal("Release started before the synthesized terminal committed")
	}
	close(executor.allowRelease)
	collectEvents(stream)
}

func TestCoordinatorCommitFailureNeverPublishesUnbackedFact(t *testing.T) {
	executor := &fakeExecutor{events: []ExecutorPayload{CompactionBoundary{MessagesBefore: 4, MessagesAfter: 2}}}
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
	if executor.releases() != 1 {
		t.Fatalf("Release calls = %d, want 1", executor.releases())
	}
}

func TestCoordinatorStartExecutorError(t *testing.T) {
	executor := &fakeExecutor{startErr: errors.New("boom")}
	coordinator := testCoordinator(executor, &fakeEffects{})

	_, err := coordinator.openSegment(context.Background(), testSegment())
	if err == nil {
		t.Fatal("openSegment must surface the executor error")
	}
	if _, ok := coordinator.registry.Get("run_1"); executor.releases() != 1 || ok {
		t.Fatal("failed executor start was not torn down")
	}
}

func TestCoordinatorPreservesSubscriptionAndCleanupFailures(t *testing.T) {
	startErr := errors.New("event subscription failed")
	cleanupErr := errors.New("executor cleanup failed")
	executor := &fakeExecutor{startErr: startErr, releaseErr: cleanupErr}
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
	consumePulledEvents(next) // drain the remaining terminal events
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
	if executor.releases() != 1 {
		t.Fatalf("Release calls = %d, want 1", executor.releases())
	}
}

func TestCoordinatorStartAfterClosePreservesCleanupFailure(t *testing.T) {
	cleanupErr := errors.New("executor cleanup failed")
	executor := &fakeExecutor{releaseErr: cleanupErr}
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
	control := &fakeExecutionPorts{}
	coordinator.waitingSubtreeCancellationPreparer = control
	coordinator.rootCancellation = executor
	sessions := &fakeRunSessions{}
	coordinator.sessionReader = sessions
	coordinator.sessionCreator = sessions
	coordinator.activeRuns = sessions
	coordinator.interrupts = sessions
	coordinator.terminations = sessions
	coordinator.runs = &fakeRunProjection{runs: map[string]run.Run{
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
	if executor.releases() != 1 {
		t.Fatalf("pump executor releases = %d, want one", executor.releases())
	}
	if len(control.released) != 0 {
		t.Fatalf("control-surface releases = %+v, want pump-only ownership", control.released)
	}
	requireCoordinatorShutdown(t, coordinator)
	consumePulledEvents(next) // drain whatever remains
}
