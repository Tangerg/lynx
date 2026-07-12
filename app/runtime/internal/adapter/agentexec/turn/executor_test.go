package turn

import (
	"context"
	"iter"
	"testing"
)

type executorFakeDispatcher struct {
	Dispatcher
	eventsHandle TurnHandle
	events       iter.Seq[Event]
	cancelHandle TurnHandle
}

func (f *executorFakeDispatcher) Events(_ context.Context, h TurnHandle) (iter.Seq[Event], error) {
	f.eventsHandle = h
	return f.events, nil
}

func (f *executorFakeDispatcher) Cancel(_ context.Context, h TurnHandle) error {
	f.cancelHandle = h
	return nil
}

// TestExecutorForwardsOpaqueHandle: the run executor asserts the opaque handle
// back to a TurnHandle and drives the dispatcher.
func TestExecutorForwardsOpaqueHandle(t *testing.T) {
	ctx := context.Background()
	handle := TurnHandle{SessionID: "ses_1", TurnID: "run_1"}
	disp := &executorFakeDispatcher{events: func(func(Event) bool) {}}
	exec := NewExecutor(disp)

	seq, err := exec.TurnEvents(ctx, handle)
	if err != nil {
		t.Fatalf("TurnEvents: %v", err)
	}
	if seq == nil || disp.eventsHandle != handle {
		t.Fatalf("events handle=%+v seq nil=%v", disp.eventsHandle, seq == nil)
	}

	if err := exec.CancelTurn(ctx, handle); err != nil {
		t.Fatalf("CancelTurn: %v", err)
	}
	if disp.cancelHandle != handle {
		t.Fatalf("cancel handle=%+v", disp.cancelHandle)
	}
}

// TestExecutorRejectsForeignHandle: a handle that is not a TurnHandle is an
// error, not a panic — the application must only hand back the executor's own.
func TestExecutorRejectsForeignHandle(t *testing.T) {
	exec := NewExecutor(&executorFakeDispatcher{})
	if _, err := exec.TurnEvents(context.Background(), "not-a-handle"); err == nil {
		t.Fatal("TurnEvents must reject a non-turn handle")
	}
	if err := exec.CancelTurn(context.Background(), 42); err == nil {
		t.Fatal("CancelTurn must reject a non-turn handle")
	}
}
