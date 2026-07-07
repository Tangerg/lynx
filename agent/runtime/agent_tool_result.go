package runtime

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
)

// materializeToolResult converts a finished sub-agent run into the tool's
// JSON string result and translates terminal failures into driver-facing
// errors.
//
// Waiting is a soft control-flow result: the child is still in flight and must
// remain discoverable for resume, so we return a structured "status:waiting"
// payload instead of treating it as an error.
//
// All other terminal outcomes are handled in one place so `agentTool.Call` only
// needs to coordinate dependencies.
func materializeToolResult(
	label string,
	agentName string,
	child *AgentProcess,
	extract func(child *AgentProcess) (any, error),
) (string, error) {
	if child == nil {
		return "", errors.New("runtime: materializeToolResult: child process is nil")
	}

	if child.Status() == core.StatusWaiting {
		// Parked for HITL: the host resumes it via process_id, so snapshot must
		// survive. Return structured state to keep the caller in control.
		return waitingResultText(agentName, child), nil
	}

	if err := child.TerminalError(); err != nil {
		return "", fmt.Errorf("%s %q (process %q): %w", label, agentName, child.ID(), err)
	}

	out, err := extract(child)
	if err != nil {
		return "", fmt.Errorf("%s %q: %w", label, agentName, err)
	}

	encoded, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("%s %q: marshal output: %w", label, agentName, err)
	}

	return string(encoded), nil
}
