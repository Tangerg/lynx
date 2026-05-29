package runtime_test

import (
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/runtime"
)

// TestSubagentTools_BuildsForExportedGoals confirms one tool per exported
// goal is built for a deployed agent.
func TestSubagentTools_BuildsForExportedGoals(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	mustDeploy(t, platform, makeBriefingAgent(false))

	tools, err := runtime.SubagentTools(platform, "briefing")
	if err != nil {
		t.Fatalf("SubagentTools: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("tools = %d, want 1", len(tools))
	}
	if got := tools[0].Definition().Name; got != "brief-goal" {
		t.Fatalf("tool name = %q, want brief-goal", got)
	}
}

func TestSubagentTools_UnknownAgentErrors(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	if _, err := runtime.SubagentTools(platform, "ghost"); err == nil {
		t.Fatal("expected error for undeployed agent")
	}
}

func TestSubagentTools_NoExportedGoalErrors(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	mustDeploy(t, platform, makeInternalAgent()) // goal without Export

	if _, err := runtime.SubagentTools(platform, "internal"); err == nil {
		t.Fatal("expected error: agent exposes no exported goal")
	}
}
