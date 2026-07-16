package runtime

import (
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
)

// TerminalError formats a non-Completed terminal status as an error. Returns
// nil when the process completed cleanly. Used by workflow builders and
// agent-as-tool wrappers to bubble up a uniform "ended in X / ended in X:
// failure" message; call sites add their own prefix context (step number /
// agent name / iteration index).
//
// Waiting is treated as a non-terminal failure here. Agent-as-tool wrappers
// that want to surface a structured "waiting" tool-result (instead of bubbling
// the error) should branch on [core.ProcessStatus] before calling
// TerminalError.
func (p *Process) TerminalError() error {
	if p == nil {
		return errors.New("process is nil")
	}
	status := p.Status()
	if status == core.StatusCompleted {
		return nil
	}
	if failure := p.Failure(); failure != nil {
		return fmt.Errorf("ended in %s: %w", status, failure)
	}
	return fmt.Errorf("ended in %s", status)
}
