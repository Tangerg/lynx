package agentexec

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
)

func TestInteractionExecutorAppliesColdWaitingDelegateCancellationWithoutDuplicateProjection(
	t *testing.T,
) {
	fixture := newWaitingDelegateFixture(t, "native-waiting-cancellation-test")
	started := fixture.start(t)
	initialEventsReady := make(chan []runs.Event, 1)
	go func() { initialEventsReady <- slices.Collect(started.Events) }()
	select {
	case events := <-initialEventsReady:
		if len(events) == 0 {
			t.Fatal("waiting Delegate produced no events")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("waiting Delegate did not park")
	}
	barrier := fixture.waitForBarrier(t, time.Second)
	if len(barrier.Pending.Interrupts) != 1 || len(barrier.Pending.Continuations) != 2 {
		t.Fatalf("waiting Delegate boundary = %#v", barrier)
	}
	ref := runs.ExecutorRef{SessionID: barrier.Pending.SessionID, ExecutorID: barrier.Pending.ExecutorID}
	if err := fixture.executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	target := barrier.Pending.Interrupts[0]
	request := runs.WaitingSubtreeCancellationRequest{
		Continuation:   waitingDelegateContinuation(barrier),
		TargetMemberID: memberIDForRun(t, barrier.Pending, target.RunID),
		Reason:         "caller canceled the waiting delegate",
	}
	prepareCtx, cancelPrepare := context.WithTimeout(t.Context(), 2*time.Second)
	prepared, err := fixture.executor.PrepareWaitingSubtreeCancellation(prepareCtx, request)
	if err != nil {
		cancelPrepare()
		t.Fatal(err)
	}
	if err := prepared.Validate(); err != nil {
		cancelPrepare()
		t.Fatal(err)
	}
	cancelPrepare()
	if err := prepared.Change.Discard(); err != nil {
		t.Fatal(err)
	}
	liveSession, err := fixture.executor.session(ref)
	if err != nil {
		t.Fatal(err)
	}
	liveSession.mu.Lock()
	boundaryAfterDiscard := liveSession.boundary
	observerAfterDiscard := liveSession.observerWasAttached
	statusAfterDiscard := liveSession.process.Status()
	liveSession.mu.Unlock()
	assertDiscardedWaitingCancellationState(
		t,
		boundaryAfterDiscard,
		observerAfterDiscard,
		isInteractionWaitingBoundary(statusAfterDiscard),
		statusAfterDiscard.String(),
	)

	prepareCtx, cancelPrepare = context.WithTimeout(t.Context(), 2*time.Second)
	prepared, err = fixture.executor.PrepareWaitingSubtreeCancellation(prepareCtx, request)
	if err != nil {
		cancelPrepare()
		t.Fatal(err)
	}
	assertPreparedWaitingCancellation(t, prepared, request.TargetMemberID, cancelPrepare)
	sequence, err := fixture.executor.Observe(context.Background(), ref)
	if err != nil {
		cancelPrepare()
		t.Fatal(err)
	}
	resumedEvents := collectInteractionEvents(sequence)
	if err := prepared.Change.Apply(runs.WaitingSubtreeResumesRunning); err != nil {
		cancelPrepare()
		t.Fatal(err)
	}
	if calls := fixture.model.Calls(); calls != 2 {
		cancelPrepare()
		t.Fatalf("provider calls after state apply = %d, want 2 before continuation activation", calls)
	}
	if err := prepared.Change.Continue(t.Context()); err != nil {
		cancelPrepare()
		t.Fatal(err)
	}
	cancelPrepare()
	var observed []runs.ExecutorEvent
	select {
	case observed = <-resumedEvents:
	case <-time.After(3 * time.Second):
		t.Fatal("root did not finish after waiting Delegate cancellation")
	}
	assertCanceledDelegateIsNotReprojected(t, observed, request.TargetMemberID)
	if fixture.model.Calls() != 3 {
		t.Fatalf("provider calls = %d, want 3 without canceled-child replay", fixture.model.Calls())
	}
	if err := fixture.executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	fixture.shutdown(t)
}

func assertDiscardedWaitingCancellationState(
	t *testing.T,
	boundary interactionBoundary,
	observerAttached bool,
	waitingStatus bool,
	statusDescription string,
) {
	t.Helper()
	if boundary != interactionBoundaryWaiting || observerAttached || !waitingStatus {
		t.Fatalf(
			"discarded subtree boundary=%d observer=%t status=%s",
			boundary, observerAttached, statusDescription,
		)
	}
}

func assertPreparedWaitingCancellation(
	t *testing.T,
	prepared runs.PreparedWaitingSubtreeCancellation,
	targetMemberID string,
	cancelPrepare context.CancelFunc,
) {
	t.Helper()
	if len(prepared.CanceledMemberIDs) == 1 &&
		prepared.CanceledMemberIDs[0] == targetMemberID &&
		len(prepared.PausedMemberIDs) == 1 &&
		prepared.PausedMemberIDs[0] == prepared.Checkpoint.RootMemberID &&
		len(prepared.PendingInterruptions) == 0 {
		return
	}
	cancelPrepare()
	t.Fatalf(
		"prepared waiting cancellation canceled=%v paused=%v interruptions=%d",
		prepared.CanceledMemberIDs,
		prepared.PausedMemberIDs,
		len(prepared.PendingInterruptions),
	)
}

func assertCanceledDelegateIsNotReprojected(
	t *testing.T,
	events []runs.ExecutorEvent,
	canceledMemberID string,
) {
	t.Helper()
	for _, event := range events {
		if event.Member.MemberID == canceledMemberID {
			t.Fatalf("canceled Delegate leaked a duplicate executor projection: %#v", event)
		}
	}
	ended := payloadsOf[runs.SegmentEnded](events)
	if len(ended) != 1 || ended[0].Reason != run.OutcomeCompleted {
		t.Fatalf("terminal events = %#v, want one completed root", ended)
	}
}

func memberIDForRun(t *testing.T, pending runs.Pending, runID string) string {
	t.Helper()
	for _, continuation := range pending.Continuations {
		if continuation.RunID == runID {
			return continuation.MemberID
		}
	}
	t.Fatalf("Run %q has no waiting member", runID)
	return ""
}

func waitingDelegateContinuation(barrier runs.TreeBarrierCommit) runs.WaitingContinuation {
	members := make([]runs.WaitingMember, 0, len(barrier.Pending.Continuations))
	for _, member := range barrier.Pending.Continuations {
		members = append(members, runs.WaitingMember{
			RunID: member.RunID, MemberID: member.MemberID,
			ParentRunID: member.Lineage.ParentRunID, SpawnedByItemID: member.Lineage.SpawnedByItemID,
			ModelSelection: member.ModelSelection, Metrics: member.Metrics,
		})
	}
	return runs.WaitingContinuation{
		SessionID: barrier.Pending.SessionID, ExecutorID: barrier.Pending.ExecutorID,
		RootRunID: barrier.Pending.RootRunID, Members: members,
		Checkpoint: barrier.Checkpoint, Capabilities: barrier.Pending.Capabilities,
		ChildRunAdmissionEnabled: barrier.Pending.Capabilities.ChildRuns,
	}
}
