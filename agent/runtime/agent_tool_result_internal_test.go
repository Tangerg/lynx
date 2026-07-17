package runtime

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
)

func TestAgentToolWaitingResultPropagatesInvalidSuspension(t *testing.T) {
	agent := core.NewAgent(core.AgentConfig{Name: "waiting-child"})
	deployment := &Deployment{agent: agent}
	process := &Process{
		id:         "child-1",
		deployment: deployment,
		state:      newProcessState(),
	}
	process.state.currentStatus = core.StatusWaiting
	process.state.pendingSuspension = &interaction.Suspension{
		ID:           "suspension-1",
		Prompt:       json.RawMessage(`{`),
		ResumeSchema: json.RawMessage(`{"type":"boolean"}`),
	}

	tool := &agentTool{deployment: deployment}
	if _, err := tool.encodeResult(process); err == nil || !strings.Contains(err.Error(), "marshal waiting process result") {
		t.Fatalf("encodeResult error = %v, want waiting process marshal error", err)
	}
}
