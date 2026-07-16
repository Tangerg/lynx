package runtime_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

// A completed sub-agent's auto-snapshot is dead weight; the agent-as-tool must
// drop it (registry + persisted snapshot), or a parent that spawns sub-agents
// leaks one orphaned snapshot row per call. After the run only the parent's
// snapshot remains — the child's was discarded. (subInput/subOutput/
// parentOutput/childAgent live in agent_tool_test.go, same package.)
func TestAgentTool_DiscardsCompletedChildSnapshot(t *testing.T) {
	store := core.NewMemoryProcessStore()
	engine := agent.MustNewEngine(runtime.Config{BuildID: "agent-tool-discard-test", ProcessStore: store, AutoSnapshot: true})
	if _, err := engine.Deploy(childAgent()); err != nil {
		t.Fatalf("deploy child: %v", err)
	}

	parent := agent.New(agent.AgentConfig{Name: "parent", Description: "calls the child", Actions: []agent.Action{agent.NewAction("invoke-child", func(ctx context.Context, _ *core.ProcessContext, in subInput) (parentOutput, error) {
		tool, _ := runtime.NewAgentTool[subInput, subOutput](engine, "child-agent")
		args, _ := json.Marshal(in)
		out, err := tool.Call(ctx, string(args))
		if err != nil {
			return parentOutput{}, err
		}
		var decoded subOutput
		if err := json.Unmarshal([]byte(out), &decoded); err != nil {
			return parentOutput{}, err
		}
		return parentOutput{Final: decoded.Doubled}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[parentOutput](core.GoalConfig{Description: "final produced"})}})
	if _, err := engine.Deploy(parent); err != nil {
		t.Fatalf("deploy parent: %v", err)
	}

	proc, err := engine.Run(t.Context(), parent,
		map[string]any{core.DefaultBindingName: subInput{Value: 21}}, core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("parent status = %s; failure=%v", proc.Status(), proc.Failure())
	}

	ids, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("store.List: %v", err)
	}
	if len(ids) != 1 || ids[0] != proc.ID() {
		t.Errorf("store snapshot ids = %v, want only the parent %q — the completed child's snapshot leaked", ids, proc.ID())
	}
}
