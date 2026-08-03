package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

type subInput struct{ Value int }
type subOutput struct{ Doubled int }
type parentOutput struct{ Final int }

type constrainedInput struct {
	Instruction string `json:"instruction" jsonschema:"minLength=3"`
}

type constrainedOutput struct{ Done bool }

// childAgent doubles its input and binds the result.
func childAgent() *core.Agent {
	return agent.New(agent.AgentConfig{Name: "child-agent", Description: "doubles its input", Actions: []agent.Action{agent.NewAction("double", func(_ context.Context, _ *core.ProcessContext, in subInput) (subOutput, error) {
		return subOutput{Doubled: in.Value * 2}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[subOutput](core.GoalConfig{Description: "doubled"})}})
}

func constrainedChildAgent() *core.Agent {
	return agent.New(agent.AgentConfig{
		Name:        "constrained-child",
		Description: "runs one constrained instruction",
		Actions: []agent.Action{agent.NewAction("run", func(_ context.Context, _ *core.ProcessContext, _ constrainedInput) (constrainedOutput, error) {
			return constrainedOutput{Done: true}, nil
		}, core.ActionConfig{})},
		Goals: []*agent.Goal{agent.NewOutputGoal[constrainedOutput](core.GoalConfig{Description: "done"})},
	})
}

// TestAsChatTool_RunsChildAndReturnsResult exercises the full loop:
// parent action body invokes the subagent tool directly, child agent
// runs, output marshals back as JSON.
func TestAsChatTool_RunsChildAndReturnsResult(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	childDeployment, err := engine.Deploy(t.Context(), childAgent())
	if err != nil {
		t.Fatalf("deploy child: %v", err)
	}

	parent := agent.New(agent.AgentConfig{Name: "parent", Description: "calls the child", Actions: []agent.Action{agent.NewAction("invoke-child", func(ctx context.Context, _ *core.ProcessContext, in subInput) (parentOutput, error) {
		tool, _ := runtime.NewAgentTool[subInput, subOutput](engine, childDeployment)
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

	if _, err := engine.Deploy(t.Context(), parent); err != nil {
		t.Fatalf("deploy parent: %v", err)
	}

	proc, err := engine.Run(
		t.Context(), parent,
		core.Input(subInput{Value: 21}),
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
	before := proc.Usage()
	if before.Actions != 2 {
		t.Fatalf("subtree actions = %d, want parent + child", before.Actions)
	}
	tree, err := engine.SnapshotTree(t.Context(), proc.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Snapshots) != 2 {
		t.Fatalf("snapshot count = %d, want complete parent-child tree", len(tree.Snapshots))
	}
	var childID string
	var childSpawnCallID string
	for _, snapshot := range tree.Snapshots {
		if snapshot.ParentID == proc.ID() {
			childID = snapshot.ID
			childSpawnCallID = snapshot.SpawnCallID
		}
	}
	if childID == "" {
		t.Fatal("snapshot tree has no child")
	}
	if childSpawnCallID == "" {
		t.Fatal("AgentTool child snapshot omitted SpawnCallID")
	}
	if _, err := engine.SnapshotTree(t.Context(), childID); err == nil || !strings.Contains(err.Error(), "not a process-tree root") {
		t.Fatalf("SnapshotTree(child) error = %v", err)
	}
	if err := engine.RemoveTree(t.Context(), childID); err == nil || !strings.Contains(err.Error(), "not a process-tree root") {
		t.Fatalf("RemoveTree(child) error = %v", err)
	}
	if err := engine.RemoveTree(t.Context(), proc.ID()); err != nil {
		t.Fatal(err)
	}
	restored, err := engine.RestoreTree(t.Context(), tree, core.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	restoredChild, ok := engine.Process(childID)
	if !ok {
		t.Fatalf("restored child %q is not registered", childID)
	}
	if restoredChild.SpawnCallID() != childSpawnCallID {
		t.Fatalf("restored child SpawnCallID = %q, want %q", restoredChild.SpawnCallID(), childSpawnCallID)
	}
	if after := restored.Usage(); after != before {
		t.Fatalf("restored subtree usage = %#v, want %#v", after, before)
	}
}

// TestAsChatTool_NoParentProcessInCtx verifies the helper rejects
// callers without core.WithProcess in ctx.
func TestAsChatTool_NoParentProcessInCtx(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	childDeployment, err := engine.Deploy(t.Context(), childAgent())
	if err != nil {
		t.Fatalf("deploy child: %v", err)
	}

	tool, _ := runtime.NewAgentTool[subInput, subOutput](engine, childDeployment)
	_, err = tool.Call(t.Context(), `{"Value":1}`)
	if err == nil || !strings.Contains(err.Error(), "no parent process in ctx") {
		t.Fatalf("expected no-parent-process error, got %v", err)
	}
}

func TestAsChatTool_EnforcesOneTypedInputContract(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	childDeployment, err := engine.Deploy(t.Context(), constrainedChildAgent())
	if err != nil {
		t.Fatal(err)
	}
	tool, err := runtime.NewAgentTool[constrainedInput, constrainedOutput](engine, childDeployment)
	if err != nil {
		t.Fatal(err)
	}
	for _, arguments := range []string{
		`{"instruction":"do work","prompt":"legacy"}`,
		`{"instruction":"x"}`,
		`{}`,
		`{"instruction":"do work"} {}`,
	} {
		_, err := tool.Call(t.Context(), arguments)
		if err == nil {
			t.Errorf("Call(%s): want typed-contract rejection", arguments)
			continue
		}
		if strings.Contains(err.Error(), "no parent process") {
			t.Errorf("Call(%s) reached Agent execution before input rejection: %v", arguments, err)
		}
	}
}

func TestAsChatTool_RejectsNilDeployment(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	if _, err := runtime.NewAgentTool[subInput, subOutput](engine, nil); err == nil {
		t.Fatal("expected error on nil deployment")
	}
}

func TestAsChatTool_RejectsForeignDeployment(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	other := agent.MustNewEngine(runtime.Config{})
	deployment, err := other.Deploy(t.Context(), childAgent())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.NewAgentTool[subInput, subOutput](engine, deployment); !errors.Is(err, runtime.ErrDeploymentNotFound) {
		t.Fatalf("NewAgentTool foreign deployment error = %v, want ErrDeploymentNotFound", err)
	}
}

func TestAsChatTool_RejectsInterfaceInput(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	childDeployment, err := engine.Deploy(t.Context(), childAgent())
	if err != nil {
		t.Fatalf("deploy child: %v", err)
	}
	if _, err := runtime.NewAgentTool[any, subOutput](engine, childDeployment); err == nil ||
		!strings.Contains(err.Error(), "input type must be concrete") {
		t.Fatalf("NewAgentTool interface input error = %v", err)
	}
}

// TestAsChatTool_DefinitionUsesAgentMetadata verifies the tool surface
// reflects the wrapped agent's name + description and a JSON schema
// derived from In.
func TestAsChatTool_DefinitionUsesAgentMetadata(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	childDeployment, err := engine.Deploy(t.Context(), childAgent())
	if err != nil {
		t.Fatalf("deploy child: %v", err)
	}
	tool, _ := runtime.NewAgentTool[subInput, subOutput](engine, childDeployment)
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
