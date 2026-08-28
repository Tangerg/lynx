package runmaintenance

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tangerg/scope/core/chat"
)

// RunningShell is one background shell still executing when a compaction ran.
type RunningShell struct {
	ID      string
	Command string
}

// LiveStateSnapshot is non-durable process state an LLM history summary cannot
// reconstruct. Durable Goal and Plan aggregates are refreshed separately before
// every model call; only running OS resources belong in this reminder.
type LiveStateSnapshot struct {
	Shells []RunningShell
}

func (l LiveStateSnapshot) empty() bool {
	return len(l.Shells) == 0
}

// LiveStateSnapshotter snapshots a session's active execution state at the moment a
// compaction rewrites its history. It is deterministic (no model call). A nil
// LiveStateSnapshotter disables the reminder.
type LiveStateSnapshotter func(ctx context.Context, sessionID string) LiveStateSnapshot

// liveStateReminder renders snap as a system-reminder message to append after a
// compaction summary, or reports false when there is nothing active to carry
// over. The tool names it points at (read_shell_output / stop_shell) are the
// stable names of the tools that own that state.
func liveStateReminder(snap LiveStateSnapshot) (chat.Message, bool) {
	if snap.empty() {
		return chat.Message{}, false
	}
	var b strings.Builder
	b.WriteString("<system-reminder>\nThe earlier conversation was summarized to save context. Execution state that was active then — and may still be — is not captured in the summary:\n")
	if len(snap.Shells) > 0 {
		b.WriteString("\nBackground shells (read their output with read_shell_output, stop them with stop_shell):")
		for _, sh := range snap.Shells {
			fmt.Fprintf(&b, "\n  - %s: %s", sh.ID, sh.Command)
		}
		b.WriteByte('\n')
	}
	b.WriteString("</system-reminder>")
	return chat.NewSystemMessage(b.String()), true
}
