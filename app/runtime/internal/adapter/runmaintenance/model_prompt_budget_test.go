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

	for _, toolResultCap := range []int{uncappedToolResults, summaryToolResultCap} {
		transcript := renderTranscript(messages, toolResultCap)
		if len(transcript) > maximumInputBytes {
			t.Errorf("renderTranscript(toolResultCap=%d) = %d bytes, want at most %d", toolResultCap, len(transcript), maximumInputBytes)
		}
	}
}
