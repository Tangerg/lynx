package agentexec

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/application/runs"
	"github.com/Tangerg/scope/app/runtime/internal/domain/interrupt"
)

func TestInteractionExecutorRestoresWaitingDelegateChildWithoutReadmission(t *testing.T) {
	fixture := newWaitingDelegateFixture(t, "interaction-waiting-delegate-test")
	started := fixture.start(t)
	initialEventsReady := make(chan []runs.Event, 1)
	go func() { initialEventsReady <- slices.Collect(started.Events) }()

	barrier := fixture.waitForBarrier(t, 2*time.Second)
	reservationsBeforeRestore, outcomesBeforeRestore := fixture.admissionCounts()
	assertWaitingDelegateBoundary(t, barrier)
	if err := fixture.executor.Release(t.Context(), runs.ExecutorRef{
		SessionID: barrier.Pending.SessionID, ExecutorID: barrier.Pending.ExecutorID,
	}); err != nil {
		t.Fatal(err)
	}

	continuation := waitingDelegateContinuation(barrier)
	ref, err := fixture.executor.StageContinuation(t.Context(), continuation)
	if err != nil {
		t.Fatal(err)
	}
	sequence, err := fixture.executor.Observe(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	eventsReady := collectInteractionEvents(sequence)
	binding := barrier.Pending.Bindings[0]
	if err := fixture.executor.BeginContinuation(t.Context(), ref, []runs.InterruptAnswer{{
		InterruptItemID: binding.InterruptItemID, MemberID: binding.MemberID,
		RequestID:  binding.RequestID,
		Resolution: interrupt.Resolution{Answers: [][]string{{"restored value"}}},
	}}, barrier.Pending.Capabilities.InterruptKinds); err != nil {
		t.Fatal(err)
	}
	var observed []runs.ExecutorEvent
	select {
	case observed = <-eventsReady:
	case <-time.After(2 * time.Second):
		t.Fatal("restored waiting Delegate did not finish")
	}
	if err := fixture.executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	reservationsAfterRestore, outcomesAfterRestore := fixture.admissionCounts()
	assertDelegateAdmissionUnchanged(
		t,
		reservationsBeforeRestore,
		outcomesBeforeRestore,
		reservationsAfterRestore,
		outcomesAfterRestore,
	)
	assertRestoredDelegateEventOrder(t, observed, binding.MemberID)
	if fixture.model.Calls() != 4 {
		t.Fatalf("provider calls = %d, want 4 without replay", fixture.model.Calls())
	}
	fixture.shutdown(t)
	select {
	case <-initialEventsReady:
	case <-time.After(time.Second):
		t.Fatal("initial waiting Delegate event stream did not close at shutdown")
	}
}

func assertWaitingDelegateBoundary(t *testing.T, barrier runs.TreeBarrierCommit) {
	t.Helper()
	if len(barrier.Pending.Continuations) != 2 || len(barrier.Pending.Interrupts) != 1 ||
		len(barrier.Pending.Bindings) != 1 || barrier.Pending.Interrupts[0].RunID != "run_child" {
		t.Fatalf("waiting Delegate boundary = %#v", barrier.Pending)
	}
	checkpointState, err := decodeInteractionCheckpointPayload(barrier.Checkpoint.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if processCount := len(checkpointState.tree.ProcessSnapshots()); processCount != 2 {
		t.Fatalf("checkpoint Process count = %d, want 2", processCount)
	}
}

func assertDelegateAdmissionUnchanged(
	t *testing.T,
	reservationsBefore,
	outcomesBefore,
	reservationsAfter,
	outcomesAfter int,
) {
	t.Helper()
	if reservationsBefore != 1 || outcomesBefore != 1 ||
		reservationsAfter != reservationsBefore || outcomesAfter != outcomesBefore {
		t.Fatalf(
			"Delegate admission changed across restore: reservations %d→%d outcomes %d→%d",
			reservationsBefore, reservationsAfter, outcomesBefore, outcomesAfter,
		)
	}
}

func assertRestoredDelegateEventOrder(t *testing.T, observed []runs.ExecutorEvent, childMemberID string) {
	t.Helper()
	childEnd, parentToolEnd, rootEnd := -1, -1, -1
	for index, event := range observed {
		switch payload := event.Payload.(type) {
		case runs.SegmentEnded:
			if event.Member.MemberID == childMemberID {
				childEnd = index
			} else if !event.Member.Child() {
				rootEnd = index
			}
		case runs.ToolCallFinished:
			if !event.Member.Child() && payload.CallID != "" {
				parentToolEnd = index
			}
		}
	}
	if childEnd < 0 || parentToolEnd <= childEnd || rootEnd <= parentToolEnd {
		t.Fatalf(
			"restored Delegate order child-end=%d parent-tool=%d root-end=%d events=%#v",
			childEnd, parentToolEnd, rootEnd, observed,
		)
	}
}
