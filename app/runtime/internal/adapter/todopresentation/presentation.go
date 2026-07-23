// Package todopresentation renders Todo domain values for model-facing prompt
// and tool responses. It is shared by the two adapters that need the same
// stable text form.
package todopresentation

import (
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
)

// Render formats a todo list for agent consumption. An empty list renders as
// an empty string.
func Render(items []todo.Item) string {
	var text strings.Builder
	for _, item := range items {
		text.WriteString(statusMark(item.Status))
		text.WriteByte(' ')
		text.WriteString(item.Content)
		text.WriteByte('\n')
		if item.BlockedReason != "" {
			text.WriteString("    blocked: ")
			text.WriteString(item.BlockedReason)
			text.WriteByte('\n')
		}
		if item.NextAction != "" {
			text.WriteString("    next: ")
			text.WriteString(item.NextAction)
			text.WriteByte('\n')
		}
	}
	return text.String()
}

func statusMark(status todo.Status) string {
	switch status {
	case todo.StatusCompleted:
		return "[x]"
	case todo.StatusInProgress:
		return "[~]"
	default:
		return "[ ]"
	}
}
