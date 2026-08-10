package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/itemfixture"
)

func TestPresentToolCallTiming(t *testing.T) {
	startedAt := time.Date(2026, 8, 4, 10, 0, 0, 0, time.UTC)
	running := presentItem(itemfixture.MustRestore(itemfixture.Input{
		ID: "item_1", RunID: "run_1", Kind: transcript.ToolCall,
		Status: transcript.ItemRunning, OccurredAt: startedAt,
		Tool: &transcript.ToolInvocation{Name: "shell"},
	}))
	if !running.CreatedAt.IsZero() || running.StartedAt != startedAt || !running.FinishedAt.IsZero() || running.DurationMillis != nil {
		t.Fatalf("running timing = started %s finished %s duration %v", running.StartedAt, running.FinishedAt, running.DurationMillis)
	}

	finishedAt := startedAt.Add(1250 * time.Millisecond)
	completed := presentItem(itemfixture.MustRestore(itemfixture.Input{
		ID: "item_1", RunID: "run_1", Kind: transcript.ToolCall,
		Status: transcript.ItemCompleted, OccurredAt: startedAt, FinishedAt: finishedAt,
		Tool: &transcript.ToolInvocation{Name: "shell"},
	}))
	if !completed.CreatedAt.IsZero() || completed.StartedAt != startedAt || completed.FinishedAt != finishedAt ||
		completed.DurationMillis == nil || *completed.DurationMillis != 1250 {
		t.Fatalf("completed timing = started %s finished %s duration %v", completed.StartedAt, completed.FinishedAt, completed.DurationMillis)
	}
	encoded, err := json.Marshal(completed)
	if err != nil {
		t.Fatal(err)
	}
	if body := string(encoded); strings.Contains(body, `"createdAt"`) || !strings.Contains(body, `"startedAt"`) {
		t.Fatalf("tool-call wire timing is not exclusive: %s", body)
	}

	message := presentItem(itemfixture.MustRestore(itemfixture.Input{
		ID: "item_2", RunID: "run_1", Kind: transcript.AgentMessage,
		Status: transcript.ItemCompleted, OccurredAt: startedAt,
	}))
	encoded, err = json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	if body := string(encoded); !strings.Contains(body, `"createdAt"`) || strings.Contains(body, `"startedAt"`) {
		t.Fatalf("message wire timing is not exclusive: %s", body)
	}
}
