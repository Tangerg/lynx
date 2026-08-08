package runs

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

func TestRetentionChargeTracksEveryVariableReplayPayload(t *testing.T) {
	const growth = 32 << 10
	largeText := strings.Repeat("x", growth)
	largeResult := tool.StringResult(largeText)

	tests := []struct {
		name  string
		small RunEvent
		large RunEvent
	}{
		{
			name:  "run",
			small: SegmentStarted{Run: transcript.Run{ID: "run"}},
			large: SegmentStarted{Run: transcript.Run{ID: "run", Detail: largeText}},
		},
		{
			name:  "item text",
			small: ItemStarted{Item: transcript.Item{ID: "item"}},
			large: ItemStarted{Item: transcript.Item{ID: "item", Text: largeText}},
		},
		{
			name:  "item media",
			small: ItemCompleted{Item: transcript.Item{ID: "item"}},
			large: ItemCompleted{Item: transcript.Item{ID: "item", Content: []transcript.ContentBlock{{Kind: transcript.ImageContent, MediaType: "image/png", Bytes: make([]byte, growth)}}}},
		},
		{
			name:  "tool result",
			small: ItemCompleted{Item: transcript.Item{Tool: &transcript.ToolInvocation{Name: "shell"}}},
			large: ItemCompleted{Item: transcript.Item{Tool: &transcript.ToolInvocation{Name: "shell", Result: &largeResult}}},
		},
		{
			name:  "state snapshot",
			small: StateSnapshot{SessionID: "session"},
			large: StateSnapshot{SessionID: "session", Plan: []PlanSnapshot{{ID: "step", Description: largeText, Status: plan.StatusPending}}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			small := test.small.retainedBytes()
			large := test.large.retainedBytes()
			if large-small < growth {
				t.Fatalf("large payload charge grew by %d bytes, want at least %d", large-small, growth)
			}
		})
	}
}

func TestNonReplayablePayloadsDoNotConsumeReplayBudget(t *testing.T) {
	if got := (SegmentProgressed{Progress: RunProgress{Activity: strings.Repeat("x", 1024)}}).retainedBytes(); got != 0 {
		t.Fatalf("SegmentProgressed retention charge = %d, want 0", got)
	}
	if got := (ItemChanged{Delta: ItemDelta{Text: strings.Repeat("x", 1024)}}).retainedBytes(); got != 0 {
		t.Fatalf("ItemChanged retention charge = %d, want 0", got)
	}
}
