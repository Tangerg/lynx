package turn

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
)

type subtreeTurnProcess struct {
	rootCanceled bool
	targets      []string
}

func (*subtreeTurnProcess) ID() string { return "process_root" }

func (*subtreeTurnProcess) Await() agentexec.TurnCompletion {
	return agentexec.TurnCompletion{Status: core.StatusRunning}
}

func (process *subtreeTurnProcess) Cancel(context.Context) error {
	process.rootCanceled = true
	return nil
}

func (process *subtreeTurnProcess) CancelSubtree(_ context.Context, processID string) error {
	process.targets = append(process.targets, processID)
	return nil
}

func (*subtreeTurnProcess) Resume(context.Context, []agentexec.SuspensionAnswer) error {
	return nil
}

func (*subtreeTurnProcess) PendingSuspensions(context.Context) ([]agentexec.PendingSuspension, error) {
	return nil, nil
}

func (*subtreeTurnProcess) Discard(context.Context) error { return nil }

func TestCancelSubtreeDoesNotClaimTheTurnLifecycle(t *testing.T) {
	process := &subtreeTurnProcess{}
	handle := TurnHandle{SessionID: "session", TurnID: "turn"}
	state := newRunningTestState(t.Context(), handle, process)
	dispatcher := &memoryDispatcher{
		turns:        map[string]*turnState{handle.TurnID: state},
		seenSessions: map[string]struct{}{},
	}

	if err := dispatcher.CancelSubtree(t.Context(), handle, "process_child"); err != nil {
		t.Fatalf("CancelSubtree: %v", err)
	}
	if len(process.targets) != 1 || process.targets[0] != "process_child" {
		t.Fatalf("subtree calls = %v, want [process_child]", process.targets)
	}
	if process.rootCanceled {
		t.Fatal("subtree cancellation called root Cancel")
	}
	if _, err := dispatcher.findTurn(handle.TurnID); err != nil {
		t.Fatalf("subtree cancellation released the owning turn: %v", err)
	}
	if state.released() {
		t.Fatal("subtree cancellation marked the owning turn released")
	}
}
