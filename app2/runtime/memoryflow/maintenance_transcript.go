package memoryflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app2/runtime/domain/agentmemory"
	conversationdomain "github.com/Tangerg/lynx/app2/runtime/domain/conversation"
)

func renderMaintenanceTranscript(
	run agentmemory.MaintenanceRun,
	records []conversationdomain.Record,
) (string, bool, error) {
	blocks := make([]string, 0, len(records))
	hasUser := false
	hasAssistant := false
	for _, record := range records {
		if record.RunID != run.RunID || record.SessionID != run.SessionID {
			return "", false, errors.New(
				"memoryflow: maintenance Conversation changed source",
			)
		}
		var message chat.Message
		if err := json.Unmarshal(record.Body, &message); err != nil {
			return "", false, fmt.Errorf(
				"memoryflow: decode maintenance Conversation: %w",
				err,
			)
		}
		block := renderMaintenanceMessage(message)
		if block == "" {
			continue
		}
		hasUser = hasUser || message.Role == chat.RoleUser
		hasAssistant = hasAssistant || message.Role == chat.RoleAssistant
		blocks = append(blocks, boundUTF8(block, maximumTranscriptBlock))
	}
	selected := boundTranscriptBlocks(blocks, maximumTranscriptBytes)
	transcript := strings.Join(selected, "\n\n")
	return transcript,
		hasUser && hasAssistant && len(transcript) >= minimumTranscriptBytes,
		nil
}

func renderMaintenanceMessage(message chat.Message) string {
	if err := message.Validate(); err != nil || message.Role == chat.RoleSystem {
		return ""
	}
	parts := make([]string, 0, len(message.Parts))
	for _, part := range message.Parts {
		switch part.Kind {
		case chat.PartText:
			parts = append(parts, part.Text)
		case chat.PartMedia:
			parts = append(parts, "[media attachment]")
		case chat.PartToolCall:
			parts = append(parts, fmt.Sprintf("[tool call: %s]", part.ToolCall.Name))
		case chat.PartToolResult:
			parts = append(parts, fmt.Sprintf(
				"[tool result: %s, error=%t]",
				part.ToolResult.Name,
				part.ToolResult.IsError,
			))
		case chat.PartReasoning:
			// Private reasoning is not durable fact evidence.
		}
	}
	content := strings.TrimSpace(strings.Join(parts, "\n"))
	if content == "" {
		return ""
	}
	return "[" + string(message.Role) + "]\n" + content
}

func boundTranscriptBlocks(blocks []string, limit int) []string {
	if len(blocks) == 0 || limit <= 0 {
		return nil
	}
	first := blocks[0]
	if len(first) >= limit {
		return []string{boundUTF8(first, limit)}
	}
	selected := []string{first}
	remaining := limit - len(first)
	tail := make([]string, 0, len(blocks)-1)
	for index := len(blocks) - 1; index > 0; index-- {
		block := blocks[index]
		required := len(block) + 2
		if required > remaining {
			continue
		}
		tail = append(tail, block)
		remaining -= required
	}
	slices.Reverse(tail)
	return append(selected, tail...)
}

func boundUTF8(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(value) <= limit {
		return value
	}
	const marker = "\n…[content elided]…\n"
	if limit <= len(marker) {
		end := limit
		for end > 0 && end < len(marker) && !utf8.RuneStart(marker[end]) {
			end--
		}
		return marker[:end]
	}
	available := max(limit-len(marker), 0)
	head := available * 3 / 4
	tail := available - head
	for head > 0 && !utf8.RuneStart(value[head]) {
		head--
	}
	tailStart := len(value) - tail
	for tailStart < len(value) && !utf8.RuneStart(value[tailStart]) {
		tailStart++
	}
	return value[:head] + marker + value[tailStart:]
}
