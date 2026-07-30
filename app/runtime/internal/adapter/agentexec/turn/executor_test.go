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

type executorFakeDispatcher struct {
	eventsHandle TurnHandle
	events       iter.Seq[runs.ExecutorEvent]
	cancelHandle TurnHandle
	cancelErr    error
	subtreeID    string
}

func (f *executorFakeDispatcher) Events(_ context.Context, h TurnHandle) (iter.Seq[runs.ExecutorEvent], error) {
	f.eventsHandle = h
	return f.events, nil
}

func (f *executorFakeDispatcher) Cancel(_ context.Context, h TurnHandle) error {
	f.cancelHandle = h
	return f.cancelErr
}

func (f *executorFakeDispatcher) CancelSubtree(_ context.Context, h TurnHandle, processID string) error {
	f.cancelHandle = h
	f.subtreeID = processID
	return f.cancelErr
}

func (*executorFakeDispatcher) InjectSteering(context.Context, TurnHandle, []transcript.ContentBlock) error {
	return nil
}
func (*executorFakeDispatcher) PrepareTurn(context.Context, runs.StartTurn) (TurnHandle, error) {
	return TurnHandle{}, nil
}
func (*executorFakeDispatcher) ActivateTurn(context.Context, TurnHandle) error { return nil }
func (*executorFakeDispatcher) Resume(context.Context, TurnHandle, []agentexec.SuspensionAnswer, []execution.InterruptKind) error {
	return nil
}
func (*executorFakeDispatcher) ProcessID(context.Context, TurnHandle) (string, error) { return "", nil }
func (*executorFakeDispatcher) Rehydrate(context.Context, runs.RehydrateTurn) (TurnHandle, error) {
	return TurnHandle{}, nil
}

// TestExecutorTranslatesTurnReference verifies the application-owned durable
// identity is translated into the dispatcher's concrete handle.
func TestExecutorTranslatesTurnReference(t *testing.T) {
	ctx := context.Background()
	handle := TurnHandle{SessionID: "ses_1", TurnID: "run_1"}
	ref := execution.TurnRef{SessionID: handle.SessionID, TurnID: handle.TurnID}
	disp := &executorFakeDispatcher{events: func(func(runs.ExecutorEvent) bool) {}}
	exec := NewExecutor(disp)

	seq, err := exec.TurnEvents(ctx, ref)
	if err != nil {
		t.Fatalf("TurnEvents: %v", err)
	}
	if seq == nil || disp.eventsHandle != handle {
		t.Fatalf("events handle=%+v seq nil=%v", disp.eventsHandle, seq == nil)
	}

	if err := exec.CancelTurn(ctx, ref); err != nil {
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
	err := mapControlError(agentexec.ErrProcessSnapshotLost)
	if !errors.Is(err, runs.ErrTurnStateLost) || !errors.Is(err, agentexec.ErrProcessSnapshotLost) {
		t.Fatalf("mapControlError = %v, want both turn-state and snapshot-loss identities", err)
	}
}

func TestExecutorMapsMissingTurnOnBothCancelPorts(t *testing.T) {
	dispatcher := &executorFakeDispatcher{cancelErr: ErrTurnNotFound}
	executor := NewExecutor(dispatcher)
	ref := execution.TurnRef{SessionID: "ses_1", TurnID: "turn_1"}

	tests := []struct {
		name   string
		cancel func() error
	}{
		{name: "segment", cancel: func() error { return executor.CancelTurn(t.Context(), ref) }},
		{name: "subtree", cancel: func() error {
			return executor.CancelSubtree(t.Context(), ref, "process_child")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.cancel(); !errors.Is(err, runs.ErrTurnNotLive) || !errors.Is(err, ErrTurnNotFound) {
				t.Fatalf("cancel error = %v, want both turn-not-live identities", err)
			}
		})
	}
}
