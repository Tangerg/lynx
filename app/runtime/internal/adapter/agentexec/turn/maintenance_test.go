package turn_test

import (
	"context"
	"sync"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/chatclient"
)

type recordingMaintenance struct {
	mu     sync.Mutex
	input  turn.BoundaryMaintenanceInput
	called bool
	result turn.BoundaryMaintenanceResult
}

func (m *recordingMaintenance) Maintain(_ context.Context, input turn.BoundaryMaintenanceInput) turn.BoundaryMaintenanceResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.input = input
	m.called = true
	return m.result
}

func (m *recordingMaintenance) snapshot() (turn.BoundaryMaintenanceInput, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.input, m.called
}

func TestTurnDelegatesCleanBoundaryMaintenanceAndPublishesCompaction(t *testing.T) {
	engine := &stubEngine{}
	maintenance := &recordingMaintenance{result: turn.BoundaryMaintenanceResult{
		Compaction: turn.CompactionResult{Compacted: true, MessagesBefore: 12, MessagesAfter: 5},
	}}
	client, err := chatclient.New(newCapturingModel(), chatclient.Config{})
	if err != nil {
		t.Fatalf("chatclient.New: %v", err)
	}
	controller, err := turn.New(turnDeps(engine, func(deps *turn.Dependencies) {
		deps.Maintenance = maintenance
		deps.ClientResolver = &fakeResolver{client: client}
	}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	handle, err := controller.PrepareTurn(t.Context(), runs.StartExecution{
		SessionID: "session", Message: "hello", Cwd: "/project", ModelSelection: testModelSelection(t, "openai", "gpt-test"),
	})
	if err != nil {
		t.Fatalf("PrepareTurn: %v", err)
	}
	events, err := controller.Events(context.Background(), handle)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if err := controller.ActivateTurn(t.Context(), handle); err != nil {
		t.Fatalf("ActivateTurn: %v", err)
	}

	var boundary *runs.CompactBoundary
	for event := range events {
		if compacted, ok := event.Payload.(runs.CompactBoundary); ok {
			boundary = &compacted
		}
	}
	if boundary == nil {
		t.Fatal("maintenance compaction did not publish CompactBoundary")
	}
	if boundary.MessagesBefore != 12 || boundary.MessagesAfter != 5 {
		t.Fatalf("compaction boundary = %+v, want 12 -> 5", *boundary)
	}

	input, called := maintenance.snapshot()
	if !called {
		t.Fatal("maintenance was not called")
	}
	if input.SessionID != "session" || input.Cwd != "/project" || input.ModelSelection.Provider() != "openai" || input.ModelSelection.Model() != "gpt-test" {
		t.Fatalf("maintenance input = %+v", input)
	}
	if input.PreCompact == nil || !input.PreCompact(t.Context()) {
		t.Fatal("maintenance did not receive the pre-compaction hook")
	}
}
