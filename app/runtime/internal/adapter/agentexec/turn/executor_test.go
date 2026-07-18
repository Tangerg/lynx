package turn

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
)

type executorFakeDispatcher struct {
	Dispatcher
	eventsHandle TurnHandle
	events       iter.Seq[Event]
	cancelHandle TurnHandle
	cancelErr    error
}

func (f *executorFakeDispatcher) Events(_ context.Context, h TurnHandle) (iter.Seq[Event], error) {
	f.eventsHandle = h
	return f.events, nil
}

func (f *executorFakeDispatcher) Cancel(_ context.Context, h TurnHandle) error {
	f.cancelHandle = h
	return f.cancelErr
}

// TestExecutorTranslatesTurnReference verifies the application-owned durable
// identity is translated into the dispatcher's concrete handle.
func TestExecutorTranslatesTurnReference(t *testing.T) {
	ctx := context.Background()
	handle := TurnHandle{SessionID: "ses_1", TurnID: "run_1"}
	ref := runs.TurnRef{SessionID: handle.SessionID, TurnID: handle.TurnID}
	disp := &executorFakeDispatcher{events: func(func(Event) bool) {}}
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
	ref := runs.TurnRef{SessionID: "ses_1", TurnID: "turn_1"}

	tests := []struct {
		name   string
		cancel func() error
	}{
		{name: "segment", cancel: func() error { return executor.CancelTurn(t.Context(), ref) }},
		{name: "control", cancel: func() error { return executor.Cancel(t.Context(), ref) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.cancel(); !errors.Is(err, runs.ErrTurnNotLive) || !errors.Is(err, ErrTurnNotFound) {
				t.Fatalf("cancel error = %v, want both turn-not-live identities", err)
			}
		})
	}
}
