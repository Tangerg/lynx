package server

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

func TestOpenSegmentCancelsTurnWhenEventsCannotSubscribe(t *testing.T) {
	errEvents := errors.New("events failed")
	turns := &eventsFailingTurns{err: errEvents}
	s := newTestServer(&stubRuntime{turns: turns})
	handle := turn.TurnHandle{SessionID: "ses_1", TurnID: "turn_1"}

	out, events, err := s.openSegment(context.Background(), "run_1", "", handle, "ses_1", nil, nil, "", "")
	if !errors.Is(err, errEvents) {
		t.Fatalf("openSegment err = %v, want %v", err, errEvents)
	}
	if out != nil || events != nil {
		t.Fatalf("openSegment returned out=%+v events=%v, want nils", out, events)
	}
	if len(turns.canceled) != 1 || turns.canceled[0] != handle {
		t.Fatalf("canceled = %+v, want [%+v]", turns.canceled, handle)
	}
	if s.runs.Contains("run_1") {
		t.Fatal("failed openSegment must not register the run")
	}
}

type eventsFailingTurns struct {
	turn.Dispatcher
	err      error
	canceled []turn.TurnHandle
}

func (t *eventsFailingTurns) Events(context.Context, turn.TurnHandle) (iter.Seq[turn.Event], error) {
	return nil, t.err
}

func (t *eventsFailingTurns) Cancel(_ context.Context, h turn.TurnHandle) error {
	t.canceled = append(t.canceled, h)
	return nil
}
