package runtime

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

func TestAgentToolRejectsUnpromotedWaitingChild(t *testing.T) {
	agent := core.NewAgent(core.AgentConfig{Name: "waiting-child"})
	deployment := &Deployment{agent: agent}
	process := &Process{
		id:         "child-1",
		deployment: deployment,
		state:      newProcessState(),
	}
	process.state.currentStatus = core.StatusWaiting

	tool := &agentTool{deployment: deployment}
	if _, err := tool.encodeResult(process); err == nil || !strings.Contains(err.Error(), "was not promoted") {
		t.Fatalf("encodeResult error = %v, want waiting-child invariant error", err)
	}
}
