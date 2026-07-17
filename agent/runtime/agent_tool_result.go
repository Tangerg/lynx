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
		return child.waitingToolResult()
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

// waitingProcessResult is the host-facing state of an external or background
// child parked on durable input. Synchronous parent-child AgentTool calls
// suspend their parent before reaching this adapter.
type waitingProcessResult struct {
	Status       taskStatus      `json:"status"`
	Agent        string          `json:"agent"`
	ProcessID    string          `json:"process_id"`
	SuspensionID string          `json:"suspension_id,omitempty"`
	Prompt       json.RawMessage `json:"prompt,omitzero"`
	ResumeSchema json.RawMessage `json:"resume_schema,omitzero"`
}

// waitingToolResult preserves the complete resumable contract. Encoding
// failures indicate corrupt runtime state and must reach the host rather than
// being downgraded to a successful result with missing suspension fields.
func (p *Process) waitingToolResult() (string, error) {
	if p == nil {
		return "", errors.New("runtime: waiting tool result: process is nil")
	}
	agent := p.agent()
	if agent == nil {
		return "", errors.New("runtime: waiting tool result: process has no deployed agent")
	}
	result := waitingProcessResult{
		Status:    taskStatusWaiting,
		Agent:     agent.Name(),
		ProcessID: p.ID(),
	}
	if suspension := p.Suspension(); suspension != nil {
		result.SuspensionID = suspension.ID
		result.Prompt = suspension.Prompt
		result.ResumeSchema = suspension.ResumeSchema
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("runtime: marshal waiting process result: %w", err)
	}
	return string(encoded), nil
}
