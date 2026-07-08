package runtime_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/hitl"
	"github.com/Tangerg/lynx/agent/runtime"
)

type subInput struct{ Value int }
type subOutput struct{ Doubled int }
type parentOutput struct{ Final int }

// childAgent doubles its input and binds the result.
func childAgent() *core.Agent {
	return agent.New("child-agent").
		Description("doubles its input").
		Actions(agent.NewAction("double",
			func(_ context.Context, _ *core.ProcessContext, in subInput) (subOutput, error) {
				return subOutput{Doubled: in.Value * 2}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[subOutput](core.Goal{Description: "doubled"})).
		Build()
}

// TestAsChatTool_RunsChildAndReturnsResult exercises the full loop:
// parent action body invokes the subagent tool directly, child agent
// runs, output marshals back as JSON.
func TestAsChatTool_RunsChildAndReturnsResult(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	if err := platform.Deploy(childAgent()); err != nil {
		t.Fatalf("deploy child: %v", err)
	}

	parent := agent.New("parent").
		Description("calls the child").
		Actions(agent.NewAction("invoke-child",
			func(ctx context.Context, _ *core.ProcessContext, in subInput) (parentOutput, error) {
				tool, _ := runtime.AsChatTool[subInput, subOutput](platform, "child-agent")
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
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[parentOutput](core.Goal{Description: "final produced"})).
		Build()

	if err := platform.Deploy(parent); err != nil {
		t.Fatalf("deploy parent: %v", err)
	}

	proc, err := platform.RunAgent(
		t.Context(), parent,
		map[string]any{core.DefaultBindingName: subInput{Value: 21}},
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("parent status = %s; failure=%v", proc.Status(), proc.Failure())
	}
	got, ok := core.ResultOfType[parentOutput](proc)
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
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	if err := platform.Deploy(childAgent()); err != nil {
		t.Fatalf("deploy child: %v", err)
	}

	tool, _ := runtime.AsChatTool[subInput, subOutput](platform, "child-agent")
	_, err := tool.Call(t.Context(), `{"Value":1}`)
	if err == nil || !strings.Contains(err.Error(), "no parent process in ctx") {
		t.Fatalf("expected no-parent-process error, got %v", err)
	}
}

// TestAsChatTool_WaitingChildSurfacesPendingAwaitableAsToolResult
// verifies the supervisor graceful-degradation path: when the child
// agent suspends for HITL, AsChatTool returns a JSON description of
// the pending request as the tool result (rather than erroring) so
// the parent's LLM can decide to switch plans.
func TestAsChatTool_WaitingChildSurfacesPendingAwaitableAsToolResult(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})

	// Child agent's only action immediately requests HITL confirmation.
	// A raw core.Action keeps this fixture minimal; typed NewAction
	// bodies can suspend on AwaitInput too (see
	// TestTypedActionAwaitInputSuspendsAndResumes).
	awaitingChild := agent.New("awaiting-child").
		Description("asks for confirmation immediately").
		Actions(&awaitForConfirmAction{}).
		Goals(agent.GoalProducing[subOutput](core.Goal{Description: "doubled"})).
		Build()
	if err := platform.Deploy(awaitingChild); err != nil {
		t.Fatalf("deploy child: %v", err)
	}

	// Parent's action body invokes the subagent tool and inspects the
	// result text directly.
	parent := agent.New("parent-graceful").
		Description("calls a child that always suspends").
		Actions(agent.NewAction("invoke",
			func(ctx context.Context, _ *core.ProcessContext, in subInput) (parentOutput, error) {
				tool, _ := runtime.AsChatTool[subInput, subOutput](platform, "awaiting-child")
				args, _ := json.Marshal(in)
				out, err := tool.Call(ctx, string(args))
				if err != nil {
					return parentOutput{}, err
				}

				var payload map[string]any
				if err := json.Unmarshal([]byte(out), &payload); err != nil {
					return parentOutput{}, err
				}
				if payload["status"] != "waiting" {
					t.Fatalf("expected status=waiting, got %v", payload["status"])
				}
				if payload["agent"] != "awaiting-child" {
					t.Fatalf("expected agent=awaiting-child, got %v", payload["agent"])
				}
				if payload["awaitable_id"] == nil || payload["awaitable_id"] == "" {
					t.Fatalf("expected awaitableId, got %v", payload["awaitable_id"])
				}
				if payload["prompt"] != "approve doubling?" {
					t.Fatalf("expected prompt='approve doubling?', got %v", payload["prompt"])
				}
				return parentOutput{Final: -1}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[parentOutput](core.Goal{Description: "final produced"})).
		Build()
	if err := platform.Deploy(parent); err != nil {
		t.Fatalf("deploy parent: %v", err)
	}

	proc, err := platform.RunAgent(
		t.Context(), parent,
		map[string]any{core.DefaultBindingName: subInput{Value: 5}},
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("parent should complete (subagent waiting is now graceful); got %s; failure=%v", proc.Status(), proc.Failure())
	}
}

// TestAsMCPTool_RunsAgentWithoutParentProcess covers the top-level
// MCP-host invocation: no parent process in ctx, AsMCPTool spins up
// a fresh process per call, returns the JSON-encoded result.
func TestAsMCPTool_RunsAgentWithoutParentProcess(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	if err := platform.Deploy(childAgent()); err != nil {
		t.Fatalf("deploy child: %v", err)
	}

	tool, _ := runtime.AsMCPTool[subInput, subOutput](platform, "child-agent")
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

// TestAsMCPTool_DefinitionUsesAgentMetadata mirrors the AsChatTool
// equivalent so MCP hosts get the same agent name + description and
// a JSON schema derived from In.
func TestAsMCPTool_DefinitionUsesAgentMetadata(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	if err := platform.Deploy(childAgent()); err != nil {
		t.Fatalf("deploy child: %v", err)
	}
	tool, _ := runtime.AsMCPTool[subInput, subOutput](platform, "child-agent")
	def := tool.Definition()
	if def.Name != "child-agent" {
		t.Fatalf("Name = %q, want child-agent", def.Name)
	}
	if def.Description != "doubles its input" {
		t.Fatalf("Description = %q, want 'doubles its input'", def.Description)
	}
	if !strings.Contains(def.InputSchema, "Value") {
		t.Fatalf("InputSchema should include In's field name; got %s", def.InputSchema)
	}
}

// TestAsChatToolFromAgent_AcceptsAgentDirectly exercises the
// platform-bypass factory: the *core.Agent is passed in directly,
// no platform.FindAgent lookup. Useful when the caller has the
// agent struct in hand but hasn't deployed it.
func TestAsChatToolFromAgent_AcceptsAgentDirectly(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	child := childAgent()
	if err := platform.Deploy(child); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	// Pass *core.Agent directly — no name lookup needed.
	tool, _ := runtime.AsChatToolFromAgent[subInput, subOutput](platform, child)
	if def := tool.Definition(); def.Name != "child-agent" {
		t.Fatalf("Name = %q, want child-agent", def.Name)
	}

	parent := agent.New("parent-direct").
		Description("calls a child via AsChatToolFromAgent").
		Actions(agent.NewAction("invoke",
			func(ctx context.Context, _ *core.ProcessContext, in subInput) (parentOutput, error) {
				args, _ := json.Marshal(in)
				out, err := tool.Call(ctx, string(args))
				if err != nil {
					return parentOutput{}, err
				}
				var decoded subOutput
				_ = json.Unmarshal([]byte(out), &decoded)
				return parentOutput{Final: decoded.Doubled}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[parentOutput](core.Goal{Description: "final"})).
		Build()
	if err := platform.Deploy(parent); err != nil {
		t.Fatalf("deploy parent: %v", err)
	}

	proc, _ := platform.RunAgent(t.Context(), parent,
		map[string]any{core.DefaultBindingName: subInput{Value: 11}}, core.ProcessOptions{})
	got, _ := core.ResultOfType[parentOutput](proc)
	if got.Final != 22 {
		t.Fatalf("Final = %d, want 22", got.Final)
	}
}

func TestAsChatToolFromAgent_RejectsNilArgs(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	for _, tc := range []struct {
		name string
		fn   func() error
	}{
		{"nil platform", func() error {
			_, err := runtime.AsChatToolFromAgent[subInput, subOutput](nil, childAgent())
			return err
		}},
		{"nil agent", func() error {
			_, err := runtime.AsChatToolFromAgent[subInput, subOutput](platform, nil)
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.fn(); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

// TestAsMCPTool_RejectsUnknownAgent matches AsChatTool's fail-fast
// boot-time behavior, surfaced as an error.
func TestAsMCPTool_RejectsUnknownAgent(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	if _, err := runtime.AsMCPTool[subInput, subOutput](platform, "missing"); err == nil {
		t.Fatal("expected error on unknown agent name")
	}
}

// TestAsChatTool_RejectsUnknownAgent ensures construction fails fast
// when the named agent isn't registered — programming errors should
// surface at boot, not on the first LLM tool call.
func TestAsChatTool_RejectsUnknownAgent(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	if _, err := runtime.AsChatTool[subInput, subOutput](platform, "missing"); err == nil {
		t.Fatal("expected error on unknown agent name")
	}
}

// TestAsChatTool_DefinitionUsesAgentMetadata verifies the tool surface
// reflects the wrapped agent's name + description and a JSON schema
// derived from In.
func TestAsChatTool_DefinitionUsesAgentMetadata(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	if err := platform.Deploy(childAgent()); err != nil {
		t.Fatalf("deploy child: %v", err)
	}
	tool, _ := runtime.AsChatTool[subInput, subOutput](platform, "child-agent")
	def := tool.Definition()
	if def.Name != "child-agent" {
		t.Fatalf("Name = %q, want child-agent", def.Name)
	}
	if def.Description != "doubles its input" {
		t.Fatalf("Description = %q, want 'doubles its input'", def.Description)
	}
	if !strings.Contains(def.InputSchema, "Value") {
		t.Fatalf("InputSchema should include In's field name; got %s", def.InputSchema)
	}
}

// awaitForConfirmAction is a raw core.Action that immediately asks
// for HITL confirmation and returns ActionWaiting so the process
// transitions to StatusWaiting. Used to exercise AsChatTool's
// waiting graceful-degrade path. Effects claim to produce subOutput
// so the goal-reachability pre-check accepts it; we never actually
// produce one because the action suspends first.
type awaitForConfirmAction struct{}

func (awaitForConfirmAction) Metadata() core.ActionMetadata {
	binding := core.NewIOBinding[subOutput](core.DefaultBindingName)
	return core.ActionMetadata{
		Name:    "confirm",
		Outputs: []core.IOBinding{binding},
		Effects: core.Effects{binding.String(): core.True},
		Cost:    core.Static(1),
		Value:   core.Static(0),
	}
}

func (awaitForConfirmAction) Execute(ctx context.Context, pc *core.ProcessContext) core.ActionStatus {
	req := hitl.NewConfirmation("approve doubling?", func(bool) core.ResponseImpact {
		return core.ImpactUnchanged
	})
	return pc.AwaitInput(ctx, req)
}
