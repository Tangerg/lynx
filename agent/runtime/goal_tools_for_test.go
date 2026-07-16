package runtime_test

import (
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/runtime"
)

// TestGoalToolsForBuildsConfiguredGoals confirms one tool per configured
// goal is built for a deployed agent.
func TestGoalToolsForBuildsConfiguredGoals(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine, makeBriefingAgent(false))

	tools, err := runtime.GoalToolsFor(engine, "briefing")
	if err != nil {
		t.Fatalf("GoalToolsFor: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("tools = %d, want 1", len(tools))
	}
	if got := tools[0].Definition().Name; got != "brief-goal" {
		t.Fatalf("tool name = %q, want brief-goal", got)
	}
}

func TestGoalToolsForRejectsUnknownAgent(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	if _, err := runtime.GoalToolsFor(engine, "ghost"); err == nil {
		t.Fatal("expected error for undeployed agent")
	}
}

func TestGoalToolsForRejectsAgentWithoutGoalTools(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine, makeInternalAgent()) // goal without Tool metadata

	if _, err := runtime.GoalToolsFor(engine, "internal"); err == nil {
		t.Fatal("expected error: agent exposes no goal tool")
	}
}
