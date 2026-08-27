// Package planpresentation renders Plan domain values for model-facing prompt
// and tool responses.
package planpresentation

import (
	"strings"

	"github.com/Tangerg/scope/app/runtime/internal/domain/plan"
)

// Render formats an ordered Plan for Agent consumption. An empty Plan renders
// as an empty string.
func Render(steps []plan.Step) string {
	var text strings.Builder
	for _, step := range steps {
		text.WriteString(statusMark(step.Status))
		text.WriteByte(' ')
		text.WriteString(step.Description)
		text.WriteByte('\n')
	}
	return text.String()
}

func statusMark(status plan.Status) string {
	switch status {
	case plan.StatusCompleted:
		return "[x]"
	case plan.StatusInProgress:
		return "[~]"
	default:
		return "[ ]"
	}
}
