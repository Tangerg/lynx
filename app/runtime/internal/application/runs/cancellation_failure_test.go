package runs

import (
	"errors"
	"strings"
	"testing"
)

func TestLiveChildCancellationReleasesClaimWhenExecutorTeardownFails(t *testing.T) {
	plan := runningChildCancellationPlan()
	plan.executor = ExecutorRef{SessionID: "session", ExecutorID: "turn_root"}
	teardownErr := errors.New("subtree teardown failed")
	control := &fakeExecutionControl{
		cancelSubtree: func(ref ExecutorRef, processID string) error {
			if ref != (ExecutorRef{SessionID: "session", ExecutorID: "turn_root"}) {
				t.Fatalf("CancelSubtree execution = %+v, want session/exec_root", ref)
			}
			if processID != plan.target.processID {
				t.Fatalf(
					"CancelSubtree process = %q, want %q",
					processID,
					plan.target.processID,
				)
			}
			return teardownErr
		},
	}
	coordinator := NewCoordinator(Dependencies{Control: control})
	live := &handle{done: make(chan struct{})}

	_, err := coordinator.cancelLiveChild(
		t.Context(),
		CancelCommand{
			RunID:         plan.target.run.ID,
			Reason:        "stop child",
			AllowChildRun: true,
		},
		plan,
		live,
	)
	if !errors.Is(err, teardownErr) {
		t.Fatalf("cancel live child error = %v, want teardown cause", err)
	}
	for _, identity := range []string{plan.target.run.ID, plan.target.processID} {
		if !strings.Contains(err.Error(), identity) {
			t.Fatalf("cancel live child error = %q, want identity %q", err, identity)
		}
	}
	if live.childCancel != nil {
		t.Fatal("executor teardown failure retained the child cancellation claim")
	}
	retry, err := live.beginChildCancellation(plan, "retry child")
	if err != nil {
		t.Fatalf("retry child cancellation claim: %v", err)
	}
	live.abortChildCancellation(retry, errors.New("test cleanup"))
}
