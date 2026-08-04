package turn

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

type executorFakeController struct {
	eventsHandle Handle
	events       iter.Seq[runs.ExecutorEvent]
	cancelHandle Handle
	cancelErr    error
	subtreeID    string
}

func (f *executorFakeController) Events(_ context.Context, h Handle) (iter.Seq[runs.ExecutorEvent], error) {
	f.eventsHandle = h
	return f.events, nil
}

func (f *executorFakeController) Cancel(_ context.Context, h Handle) error {
	f.cancelHandle = h
	return f.cancelErr
}

func (f *executorFakeController) CancelSubtree(_ context.Context, h Handle, processID string) error {
	f.cancelHandle = h
	f.subtreeID = processID
	return f.cancelErr
}

func (*executorFakeController) InjectSteering(context.Context, Handle, []transcript.ContentBlock) error {
	return nil
}
func (*executorFakeController) PrepareTurn(context.Context, runs.StartExecution) (Handle, error) {
	return Handle{}, nil
}
func (*executorFakeController) ActivateTurn(context.Context, Handle) error { return nil }
func (*executorFakeController) Resume(context.Context, Handle, []agentexec.SuspensionAnswer, []execution.InterruptKind) error {
	return nil
}
func (*executorFakeController) ProcessID(context.Context, Handle) (string, error) { return "", nil }
func (*executorFakeController) Rehydrate(context.Context, runs.RehydrateExecution) (Handle, error) {
	return Handle{}, nil
}

// TestExecutorTranslatesReference verifies the application-owned durable
// identity is translated into the controller's concrete handle.
func TestExecutorTranslatesReference(t *testing.T) {
	ctx := context.Background()
	handle := Handle{SessionID: "ses_1", TurnID: "run_1"}
	ref := execution.ExecutorRef{SessionID: handle.SessionID, ExecutorID: handle.TurnID}
	disp := &executorFakeController{events: func(func(runs.ExecutorEvent) bool) {}}
	exec := NewExecutor(disp)

	seq, err := exec.Events(ctx, ref)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if seq == nil || disp.eventsHandle != handle {
		t.Fatalf("events handle=%+v seq nil=%v", disp.eventsHandle, seq == nil)
	}

	if err := exec.CancelExecution(ctx, ref); err != nil {
		t.Fatalf("CancelTurn: %v", err)
	}
	if disp.cancelHandle != handle {
		t.Fatalf("cancel handle=%+v", disp.cancelHandle)
	}
	if err := exec.CancelSubtree(ctx, ref, "process_child"); err != nil {
		t.Fatalf("CancelSubtree: %v", err)
	}
	if disp.cancelHandle != handle || disp.subtreeID != "process_child" {
		t.Fatalf("subtree cancel handle=%+v process=%q", disp.cancelHandle, disp.subtreeID)
	}
}

func TestExecutorMapsLostProcessSnapshot(t *testing.T) {
	err := mapControlError(agentexec.ErrExecutorCheckpointLost)
	if !errors.Is(err, runs.ErrExecutorStateLost) || !errors.Is(err, agentexec.ErrExecutorCheckpointLost) {
		t.Fatalf("mapControlError = %v, want both turn-state and snapshot-loss identities", err)
	}
}

func TestExecutorMapsMissingTurnOnBothCancelPorts(t *testing.T) {
	controller := &executorFakeController{cancelErr: ErrTurnNotFound}
	executor := NewExecutor(controller)
	ref := execution.ExecutorRef{SessionID: "ses_1", ExecutorID: "turn_1"}

	tests := []struct {
		name   string
		cancel func() error
	}{
		{name: "segment", cancel: func() error { return executor.CancelExecution(t.Context(), ref) }},
		{name: "subtree", cancel: func() error {
			return executor.CancelSubtree(t.Context(), ref, "process_child")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.cancel(); !errors.Is(err, runs.ErrExecutorNotLive) || !errors.Is(err, ErrTurnNotFound) {
				t.Fatalf("cancel error = %v, want both turn-not-live identities", err)
			}
		})
	}
}
