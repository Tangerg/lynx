package runmaintenance

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
)

func TestMaintenanceModelTranscriptHasAggregateInputBound(t *testing.T) {
	const maximumInputBytes = 384 * 1024

	largeResult := strings.Repeat("x", 8*1024)
	messages := make([]chat.Message, 0, 128)
	for index := range 128 {
		messages = append(messages, chat.NewToolMessage(chat.ToolResult{
			ID:     "call",
			Name:   "shell",
			Result: largeResult + string(rune('a'+index%26)),
		}))
	}

	transcript := renderTranscript(messages)
	if len(transcript) > maximumInputBytes {
		t.Errorf("renderTranscript = %d bytes, want at most %d", len(transcript), maximumInputBytes)
	}
	if measured := transcriptBytes(messages); measured <= len(transcript) {
		t.Fatalf("raw transcript measurement = %d, want greater than bounded rendering %d", measured, len(transcript))
	}
	if tokens := estimateTokens(messages); tokens <= len(transcript)/charsPerToken {
		t.Fatalf("compaction estimate = %d, want raw footprint rather than bounded rendering", tokens)
	}
}
