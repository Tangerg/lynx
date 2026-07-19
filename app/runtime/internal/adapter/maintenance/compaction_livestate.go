package maintenance

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/chat"
)

// RunningShell is one background shell still executing when a compaction ran.
type RunningShell struct {
	ID      string
	Command string
}

// LiveStateSnapshot is the still-active execution state an LLM history summary
// silently drops — background jobs and in-progress tasks the model started
// before its earlier turns were folded into a summary. Reminding the model of it
// keeps it from forgetting a running command or an unfinished plan across a
// compaction.
type LiveStateSnapshot struct {
	Shells []RunningShell
	Todos  []string // in-progress task descriptions
}

func (s LiveStateSnapshot) empty() bool {
	return len(s.Shells) == 0 && len(s.Todos) == 0
}

// LiveStateFunc snapshots a session's active execution state at the moment a
// compaction rewrites its history. It is deterministic (no model call). A nil
// LiveStateFunc disables the reminder.
type LiveStateFunc func(ctx context.Context, sessionID string) LiveStateSnapshot

// liveStateReminder renders snap as a system-reminder message to append after a
// compaction summary, or reports false when there is nothing active to carry
// over. The tool names it points at (shell_output / shell_kill / todo_write) are
// the stable names of the tools that produced the state.
func liveStateReminder(snap LiveStateSnapshot) (chat.Message, bool) {
	if snap.empty() {
		return chat.Message{}, false
	}
	var b strings.Builder
	b.WriteString("<system-reminder>\nThe earlier conversation was summarized to save context. Execution state that was active then — and may still be — is not captured in the summary:\n")
	if len(snap.Shells) > 0 {
		b.WriteString("\nBackground shells (read their output with shell_output, stop them with shell_kill):")
		for _, sh := range snap.Shells {
			fmt.Fprintf(&b, "\n  - %s: %s", sh.ID, sh.Command)
		}
		b.WriteByte('\n')
	}
	if len(snap.Todos) > 0 {
		b.WriteString("\nIn-progress tasks from your todo_write list:")
		for _, task := range snap.Todos {
			fmt.Fprintf(&b, "\n  - %s", task)
		}
		b.WriteByte('\n')
	}
	b.WriteString("</system-reminder>")
	return chat.NewSystemMessage(b.String()), true
}
