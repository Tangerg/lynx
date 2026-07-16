package runtime

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
)

// encodeResult converts a finished sub-agent run into the tool's
// JSON string result and translates terminal failures into driver-facing
// errors.
//
// Waiting is an external-host control-flow result for standalone and
// background tools. Synchronous parent-child AgentTool calls intercept it
// earlier and suspend the parent at the original tool-call checkpoint.
//
// All other terminal outcomes are handled in one place so [agentTool.Call] only
// needs to coordinate dependencies.
func (t *agentTool) encodeResult(child *Process) (string, error) {
	if child == nil {
		return "", errors.New("runtime.agentTool.encodeResult: child process is nil")
	}
	agentName := t.deployment.agent.Name()

	if child.Status() == core.StatusWaiting {
		return child.waitingToolResult(), nil
	}

	if err := child.TerminalError(); err != nil {
		return "", fmt.Errorf("%s %q (process %q): %w", t.label, agentName, child.ID(), err)
	}

	out, err := t.result(child)
	if err != nil {
		return "", fmt.Errorf("%s %q: %w", t.label, agentName, err)
	}

	encoded, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("%s %q: marshal output: %w", t.label, agentName, err)
	}

	return string(encoded), nil
}
