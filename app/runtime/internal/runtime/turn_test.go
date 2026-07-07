package runtime

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

type turnRuntimeDispatcher struct {
	turn.Dispatcher

	startReq    turn.StartTurnRequest
	startHandle turn.TurnHandle

	eventsHandle turn.TurnHandle
	events       iter.Seq[turn.Event]

	steeringHandle  turn.TurnHandle
	steeringMessage string

	resumeHandle     turn.TurnHandle
	resumeResolution interrupts.Resolution

	rehydrateReq    turn.RehydrateRequest
	rehydrateHandle turn.TurnHandle

	cancelHandle turn.TurnHandle

	processHandle turn.TurnHandle
	processID     string

	interruptKinds []string
}

func (s *turnRuntimeDispatcher) StartTurn(_ context.Context, req turn.StartTurnRequest) (turn.TurnHandle, error) {
	s.startReq = req
	return s.startHandle, nil
}

func (s *turnRuntimeDispatcher) Events(_ context.Context, handle turn.TurnHandle) (iter.Seq[turn.Event], error) {
	s.eventsHandle = handle
	return s.events, nil
}

func (s *turnRuntimeDispatcher) InjectSteering(_ context.Context, handle turn.TurnHandle, message string) error {
	s.steeringHandle = handle
	s.steeringMessage = message
	return nil
}

func (s *turnRuntimeDispatcher) Resume(_ context.Context, handle turn.TurnHandle, resolution interrupts.Resolution) error {
	s.resumeHandle = handle
	s.resumeResolution = resolution
	return nil
}

func (s *turnRuntimeDispatcher) Rehydrate(_ context.Context, req turn.RehydrateRequest) (turn.TurnHandle, error) {
	s.rehydrateReq = req
	return s.rehydrateHandle, nil
}

func (s *turnRuntimeDispatcher) Cancel(_ context.Context, handle turn.TurnHandle) error {
	s.cancelHandle = handle
	return nil
}

func (s *turnRuntimeDispatcher) ProcessID(_ context.Context, handle turn.TurnHandle) (string, error) {
	s.processHandle = handle
	return s.processID, nil
}

func (s *turnRuntimeDispatcher) SetInterruptKinds(kinds []string) {
	s.interruptKinds = append([]string(nil), kinds...)
}

func TestRuntimeTurnFacade(t *testing.T) {
	ctx := context.Background()
	handle := turn.TurnHandle{SessionID: "ses_1", TurnID: "run_1"}
	events := func(yield func(turn.Event) bool) {}
	svc := &turnRuntimeDispatcher{
		startHandle:     handle,
		events:          events,
		rehydrateHandle: turn.TurnHandle{SessionID: "ses_1", TurnID: "run_resumed"},
		processID:       "proc_1",
	}
	rt := &Runtime{turns: svc}

	gotHandle, err := rt.StartTurn(ctx, turn.StartTurnRequest{SessionID: "ses_1", Message: "hello"})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	if gotHandle != handle || svc.startReq.Message != "hello" {
		t.Fatalf("start handle=%+v req=%+v", gotHandle, svc.startReq)
	}

	gotEvents, err := rt.TurnEvents(ctx, handle)
	if err != nil {
		t.Fatalf("TurnEvents: %v", err)
	}
	if gotEvents == nil || svc.eventsHandle != handle {
		t.Fatalf("events handle=%+v events nil=%v", svc.eventsHandle, gotEvents == nil)
	}

	if err := rt.InjectTurnSteering(ctx, handle, "wait"); err != nil {
		t.Fatalf("InjectTurnSteering: %v", err)
	}
	if svc.steeringHandle != handle || svc.steeringMessage != "wait" {
		t.Fatalf("steering handle=%+v message=%q", svc.steeringHandle, svc.steeringMessage)
	}

	resolution := interrupts.Resolution{Approved: true}
	if err := rt.ResumeTurn(ctx, handle, resolution); err != nil {
		t.Fatalf("ResumeTurn: %v", err)
	}
	if svc.resumeHandle != handle || !svc.resumeResolution.Approved {
		t.Fatalf("resume handle=%+v resolution=%+v", svc.resumeHandle, svc.resumeResolution)
	}

	req := turn.RehydrateRequest{SessionID: "ses_1", ProcessID: "proc_1", Approved: true}
	gotRehydrated, err := rt.RehydrateTurn(ctx, req)
	if err != nil {
		t.Fatalf("RehydrateTurn: %v", err)
	}
	if gotRehydrated.TurnID != "run_resumed" || svc.rehydrateReq.ProcessID != "proc_1" {
		t.Fatalf("rehydrated=%+v req=%+v", gotRehydrated, svc.rehydrateReq)
	}

	if err := rt.CancelTurn(ctx, handle); err != nil {
		t.Fatalf("CancelTurn: %v", err)
	}
	if svc.cancelHandle != handle {
		t.Fatalf("cancel handle=%+v", svc.cancelHandle)
	}

	processID, err := rt.TurnProcessID(ctx, handle)
	if err != nil {
		t.Fatalf("TurnProcessID: %v", err)
	}
	if processID != "proc_1" || svc.processHandle != handle {
		t.Fatalf("processID=%q handle=%+v", processID, svc.processHandle)
	}

	rt.SetTurnInterruptKinds([]string{"approval", "question"})
	if len(svc.interruptKinds) != 2 || svc.interruptKinds[0] != "approval" || svc.interruptKinds[1] != "question" {
		t.Fatalf("interrupt kinds = %+v", svc.interruptKinds)
	}
}

func TestRuntimeStartTurnPersistsExplicitModelBeforeDispatch(t *testing.T) {
	ctx := context.Background()
	handle := turn.TurnHandle{SessionID: "ses_1", TurnID: "run_1"}
	turns := &turnRuntimeDispatcher{startHandle: handle}
	sessions := &sessionRuntimeStore{}
	rt := &Runtime{turns: turns, session: sessions}

	gotHandle, err := rt.StartTurn(ctx, turn.StartTurnRequest{
		SessionID: "ses_1",
		Message:   "hello",
		Provider:  "anthropic",
		Model:     "claude-opus-4-8",
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	if gotHandle != handle {
		t.Fatalf("handle = %+v, want %+v", gotHandle, handle)
	}
	if sessions.model != ([2]string{"ses_1", "claude-opus-4-8"}) {
		t.Fatalf("session model = %v", sessions.model)
	}
	if turns.startReq.Model != "claude-opus-4-8" || turns.startReq.Provider != "anthropic" {
		t.Fatalf("start req = %+v", turns.startReq)
	}
}

func TestRuntimeStartTurnDoesNotDispatchWhenModelPersistenceFails(t *testing.T) {
	ctx := context.Background()
	fail := errors.New("store failed")
	turns := &turnRuntimeDispatcher{}
	sessions := &sessionRuntimeStore{modelErr: fail}
	rt := &Runtime{turns: turns, session: sessions}

	if _, err := rt.StartTurn(ctx, turn.StartTurnRequest{SessionID: "ses_1", Message: "hello", Model: "claude-opus-4-8"}); !errors.Is(err, fail) {
		t.Fatalf("StartTurn err = %v, want store failure", err)
	}
	if turns.startReq.SessionID != "" {
		t.Fatalf("turn dispatched despite model persistence failure: %+v", turns.startReq)
	}
}
