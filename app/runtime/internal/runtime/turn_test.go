package runtime

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
)

type turnRuntimeDispatcher struct {
	turn.Dispatcher

	processHandle turn.TurnHandle
	processID     string
}

func (s *turnRuntimeDispatcher) ProcessID(_ context.Context, handle turn.TurnHandle) (string, error) {
	s.processHandle = handle
	return s.processID, nil
}

// TestRuntimeTurnProcessID: the facade resolves a parked turn's persisted process
// id through the dispatcher (the one turn touchpoint the run-segment committer
// still reads off the facade).
func TestRuntimeTurnProcessID(t *testing.T) {
	handle := turn.TurnHandle{SessionID: "ses_1", TurnID: "run_1"}
	svc := &turnRuntimeDispatcher{processID: "proc_1"}
	rt := &Runtime{turns: svc}

	processID, err := rt.TurnProcessID(context.Background(), handle)
	if err != nil {
		t.Fatalf("TurnProcessID: %v", err)
	}
	if processID != "proc_1" || svc.processHandle != handle {
		t.Fatalf("processID=%q handle=%+v", processID, svc.processHandle)
	}
}
