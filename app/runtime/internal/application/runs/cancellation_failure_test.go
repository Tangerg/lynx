package runs

import (
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

func TestLiveChildCancellationReleasesClaimWhenExecutorTeardownFails(t *testing.T) {
	plan := runningChildCancellationPlan()
	plan.turn = execution.TurnRef{SessionID: "session", TurnID: "turn_root"}
	teardownErr := errors.New("subtree teardown failed")
	turns := &fakeTurnControl{
		cancelSubtree: func(ref execution.TurnRef, processID string) error {
			if ref != (execution.TurnRef{SessionID: "session", TurnID: "turn_root"}) {
				t.Fatalf("CancelSubtree turn = %+v, want session/turn_root", ref)
			}
			if processID != plan.target.source.ProcessID {
				t.Fatalf(
					"CancelSubtree process = %q, want %q",
					processID,
					plan.target.source.ProcessID,
				)
			}
			return teardownErr
		},
	}
	coordinator := NewCoordinator(Dependencies{Turns: turns})
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
	for _, identity := range []string{plan.target.run.ID, plan.target.source.ProcessID} {
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
