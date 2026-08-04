package server

import (
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

func TestPresentToolCallTiming(t *testing.T) {
	startedAt := time.Date(2026, 8, 4, 10, 0, 0, 0, time.UTC)
	running := presentItem(transcript.Item{
		ID: "item_1", RunID: "run_1", Kind: transcript.ToolCall,
		Status: transcript.ItemRunning, CreatedAt: startedAt,
		Tool: &transcript.ToolInvocation{Name: "shell"},
	})
	if running.StartedAt != startedAt || !running.FinishedAt.IsZero() || running.DurationMs != nil {
		t.Fatalf("running timing = started %s finished %s duration %v", running.StartedAt, running.FinishedAt, running.DurationMs)
	}

	finishedAt := startedAt.Add(1250 * time.Millisecond)
	completed := presentItem(transcript.Item{
		ID: "item_1", RunID: "run_1", Kind: transcript.ToolCall,
		Status: transcript.ItemCompleted, CreatedAt: startedAt, FinishedAt: finishedAt,
		Tool: &transcript.ToolInvocation{Name: "shell"},
	})
	if completed.StartedAt != startedAt || completed.FinishedAt != finishedAt ||
		completed.DurationMs == nil || *completed.DurationMs != 1250 {
		t.Fatalf("completed timing = started %s finished %s duration %v", completed.StartedAt, completed.FinishedAt, completed.DurationMs)
	}
}
