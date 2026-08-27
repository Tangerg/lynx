package render

import (
	"fmt"
	"io"

	"github.com/Tangerg/scope/app/cli/internal/agent"
)

// WriteSessionTranscript renders every durable block in an authoritative
// session snapshot. It intentionally has no live run scope: a saved transcript
// may contain multiple root runs and their descendants.
func WriteSessionTranscript(w io.Writer, snapshot agent.SessionSnapshot) error {
	if err := snapshot.Validate(); err != nil {
		return fmt.Errorf("render session transcript: %w", err)
	}
	renderer := NewText(w)
	renderer.scope.ensureMembers()
	for _, run := range snapshot.Runs {
		renderer.scope.members[run.ID] = run.Lineage
	}
	for _, block := range snapshot.Transcript {
		renderer.finish(block)
	}
	return renderer.Close()
}
