package runtime_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

type subInput struct{ Value int }
type subOutput struct{ Doubled int }
type parentOutput struct{ Final int }

// childAgent doubles its input and binds the result.
func childAgent() *core.Agent {
	return agent.New(agent.AgentConfig{Name: "child-agent", Description: "doubles its input", Actions: []agent.Action{agent.NewAction("double", func(_ context.Context, _ *core.ProcessContext, in subInput) (subOutput, error) {
		return subOutput{Doubled: in.Value * 2}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[subOutput](core.GoalConfig{Description: "doubled"})}})
}

// TestAsChatTool_RunsChildAndReturnsResult exercises the full loop:
// parent action body invokes the subagent tool directly, child agent
// runs, output marshals back as JSON.
func TestAsChatTool_RunsChildAndReturnsResult(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
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

	proc, err := engine.Run(
		t.Context(), parent,
		map[string]any{core.DefaultBindingName: subInput{Value: 21}},
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("parent status = %s; failure=%v", proc.Status(), proc.Failure())
	}
	got, ok := core.Result[parentOutput](proc)
	if !ok {
		t.Fatal("no parentOutput produced")
	}
	if got.Final != 42 {
		t.Fatalf("Final = %d, want 42", got.Final)
	}
}

// TestAsChatTool_NoParentProcessInCtx verifies the helper rejects
// callers without core.WithProcess in ctx.
func TestAsChatTool_NoParentProcessInCtx(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	if _, err := engine.Deploy(childAgent()); err != nil {
		t.Fatalf("deploy child: %v", err)
	}

	tool, _ := runtime.NewAgentTool[subInput, subOutput](engine, "child-agent")
	_, err := tool.Call(t.Context(), `{"Value":1}`)
	if err == nil || !strings.Contains(err.Error(), "no parent process in ctx") {
		t.Fatalf("expected no-parent-process error, got %v", err)
	}
}

// TestAsMCPTool_RunsAgentWithoutParentProcess covers the top-level
// MCP-host invocation: no parent process in ctx, NewStandaloneAgentTool spins up
// a fresh process per call, returns the JSON-encoded result.
func TestAsMCPTool_RunsAgentWithoutParentProcess(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	if _, err := engine.Deploy(childAgent()); err != nil {
		t.Fatalf("deploy child: %v", err)
	}

	tool, _ := runtime.NewStandaloneAgentTool[subInput, subOutput](engine, "child-agent")
	out, err := tool.Call(t.Context(), `{"Value":21}`)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	var got subOutput
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Doubled != 42 {
		t.Fatalf("Doubled = %d, want 42", got.Doubled)
	}
}

// TestAsMCPTool_DefinitionUsesAgentMetadata mirrors the NewAgentTool
// equivalent so MCP hosts get the same agent name + description and
// a JSON schema derived from In.
func TestAsMCPTool_DefinitionUsesAgentMetadata(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	if _, err := engine.Deploy(childAgent()); err != nil {
		t.Fatalf("deploy child: %v", err)
	}
	tool, _ := runtime.NewStandaloneAgentTool[subInput, subOutput](engine, "child-agent")
	def := tool.Definition()
	if def.Name != "child-agent" {
		t.Fatalf("Name = %q, want child-agent", def.Name)
	}
	if def.Description != "doubles its input" {
		t.Fatalf("Description = %q, want 'doubles its input'", def.Description)
	}
	if !strings.Contains(string(def.InputSchema), "Value") {
		t.Fatalf("InputSchema should include In's field name; got %s", def.InputSchema)
	}
	def.InputSchema[0] = '['
	if got := tool.Definition().InputSchema[0]; got != '{' {
		t.Fatalf("mutating returned definition changed agent tool schema prefix to %q", got)
	}
}

// TestAsMCPTool_RejectsUnknownAgent matches NewAgentTool's fail-fast
// boot-time behavior, surfaced as an error.
func TestAsMCPTool_RejectsUnknownAgent(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	if _, err := runtime.NewStandaloneAgentTool[subInput, subOutput](engine, "missing"); err == nil {
		t.Fatal("expected error on unknown agent name")
	}
}

// TestAsChatTool_RejectsUnknownAgent ensures construction fails fast
// when the named agent isn't registered — programming errors should
// surface at boot, not on the first LLM tool call.
func TestAsChatTool_RejectsUnknownAgent(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	if _, err := runtime.NewAgentTool[subInput, subOutput](engine, "missing"); err == nil {
		t.Fatal("expected error on unknown agent name")
	}
}

// TestAsChatTool_DefinitionUsesAgentMetadata verifies the tool surface
// reflects the wrapped agent's name + description and a JSON schema
// derived from In.
func TestAsChatTool_DefinitionUsesAgentMetadata(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	if _, err := engine.Deploy(childAgent()); err != nil {
		t.Fatalf("deploy child: %v", err)
	}
	tool, _ := runtime.NewAgentTool[subInput, subOutput](engine, "child-agent")
	def := tool.Definition()
	if def.Name != "child-agent" {
		t.Fatalf("Name = %q, want child-agent", def.Name)
	}
	if def.Description != "doubles its input" {
		t.Fatalf("Description = %q, want 'doubles its input'", def.Description)
	}
	if !strings.Contains(string(def.InputSchema), "Value") {
		t.Fatalf("InputSchema should include In's field name; got %s", def.InputSchema)
	}
}
