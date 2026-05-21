package runtime_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

type scopeRaw struct{ Text string }
type scopeMid struct{ V int }
type scopeOut struct{ Result int }

// agentA contributes the first half of the joint plan: word → scopeMid.
func scopeAgentA() *core.Agent {
	return agent.New("scope:A").
		Actions(agent.NewAction("A:tokenize",
			func(ctx context.Context, pc *core.ProcessContext, in scopeRaw) (scopeMid, error) {
				return scopeMid{V: len(in.Text)}, nil
			},
			core.ActionConfig{},
		)).
		Build()
}

// agentB contributes the second half: scopeMid → scopeOut.
func scopeAgentB() *core.Agent {
	return agent.New("scope:B").
		Actions(agent.NewAction("B:double",
			func(ctx context.Context, pc *core.ProcessContext, in scopeMid) (scopeOut, error) {
				return scopeOut{Result: in.V * 2}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[scopeOut](core.Goal{Description: "joint output"})).
		Build()
}

// TestRunInScope_CrossAgentPlanning is the headline case: two agents
// each carry one action and (in B's case) the goal. RunInScope unions
// them and the planner picks the path A:tokenize → B:double.
func TestRunInScope_CrossAgentPlanning(t *testing.T) {
	platform := agent.NewPlatform(&runtime.PlatformConfig{})

	proc, err := platform.RunInScope(
		context.Background(),
		runtime.ScopeRun{
			Name:   "scope:joint",
			Agents: []*core.Agent{scopeAgentA(), scopeAgentB()},
		},
		map[string]any{core.DefaultBindingName: scopeRaw{Text: "lynx"}},
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("RunInScope: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %s, failure = %v", proc.Status(), proc.Failure())
	}
	got, ok := core.ResultOfType[scopeOut](proc)
	if !ok {
		t.Fatalf("no scopeOut in result")
	}
	if got.Result != 8 { // len("lynx") * 2
		t.Fatalf("Result = %d, want 8", got.Result)
	}
	if len(proc.History()) != 2 {
		t.Fatalf("history length = %d, want 2 (one action from each agent)", len(proc.History()))
	}
}

// TestRunInScope_ReusesSyntheticAgent runs the same scope twice with
// the same Name and verifies the synthetic agent is deployed exactly
// once (look-up returns the existing entry on the second call).
func TestRunInScope_ReusesSyntheticAgent(t *testing.T) {
	platform := agent.NewPlatform(&runtime.PlatformConfig{})

	cfg := runtime.ScopeRun{
		Name:   "scope:reuse",
		Agents: []*core.Agent{scopeAgentA(), scopeAgentB()},
	}
	for i := 0; i < 2; i++ {
		if _, err := platform.RunInScope(
			context.Background(), cfg,
			map[string]any{core.DefaultBindingName: scopeRaw{Text: "ab"}},
			core.ProcessOptions{},
		); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	scoped, ok := platform.FindAgent("scope:reuse")
	if !ok {
		t.Fatal("scope agent not registered after RunInScope")
	}
	if scoped.Name != "scope:reuse" {
		t.Fatalf("scope agent name = %q", scoped.Name)
	}
	// Only the synthetic scope agent should be deployed — the two
	// participant agents were *never* handed to Platform.Deploy.
	if len(platform.Agents()) != 1 {
		t.Fatalf("agents deployed = %d, want 1 (only synthetic scope)", len(platform.Agents()))
	}
}

func TestRunInScope_EmptyName(t *testing.T) {
	platform := agent.NewPlatform(&runtime.PlatformConfig{})
	_, err := platform.RunInScope(
		context.Background(),
		runtime.ScopeRun{Agents: []*core.Agent{scopeAgentA()}},
		nil, core.ProcessOptions{},
	)
	if err == nil || !strings.Contains(err.Error(), "Name must not be empty") {
		t.Fatalf("want Name-empty error, got %v", err)
	}
}

func TestRunInScope_EmptyAgents(t *testing.T) {
	platform := agent.NewPlatform(&runtime.PlatformConfig{})
	_, err := platform.RunInScope(
		context.Background(),
		runtime.ScopeRun{Name: "empty"},
		nil, core.ProcessOptions{},
	)
	if err == nil || !strings.Contains(err.Error(), "Agents must not be empty") {
		t.Fatalf("want Agents-empty error, got %v", err)
	}
}

func TestRunInScope_NilAgentEntry(t *testing.T) {
	platform := agent.NewPlatform(&runtime.PlatformConfig{})
	_, err := platform.RunInScope(
		context.Background(),
		runtime.ScopeRun{
			Name:   "with-nil",
			Agents: []*core.Agent{scopeAgentA(), nil},
		},
		nil, core.ProcessOptions{},
	)
	if err == nil || !strings.Contains(err.Error(), "Agents[1] is nil") {
		t.Fatalf("want nil-entry error, got %v", err)
	}
}

// TestRunInScope_DuplicateActionNameAcrossAgents verifies that
// collisions across the union are rejected by Deploy (the synthetic
// agent must satisfy core.ValidateAgent). The user is responsible for
// prefixing names per agent to avoid this.
func TestRunInScope_DuplicateActionNameAcrossAgents(t *testing.T) {
	dup := func(name string) *core.Agent {
		return agent.New(name).
			Actions(agent.NewAction("same-name",
				func(ctx context.Context, pc *core.ProcessContext, in scopeRaw) (scopeOut, error) {
					return scopeOut{}, nil
				},
				core.ActionConfig{},
			)).
			Goals(agent.GoalProducing[scopeOut](core.Goal{Description: "ok"})).
			Build()
	}

	platform := agent.NewPlatform(&runtime.PlatformConfig{})
	_, err := platform.RunInScope(
		context.Background(),
		runtime.ScopeRun{
			Name:   "scope:dup",
			Agents: []*core.Agent{dup("agent-1"), dup("agent-2")},
		},
		nil, core.ProcessOptions{},
	)
	if err == nil || !strings.Contains(err.Error(), `duplicate action name "same-name"`) {
		t.Fatalf("want duplicate-name error, got %v", err)
	}
}

func TestBuildScopeAgent_UnionsCapabilities(t *testing.T) {
	scope := runtime.BuildScopeAgent(runtime.ScopeRun{
		Name:        "inspect",
		Description: "custom",
		Agents:      []*core.Agent{scopeAgentA(), scopeAgentB()},
	})
	if scope.Name != "inspect" {
		t.Fatalf("Name = %q", scope.Name)
	}
	if scope.Description != "custom" {
		t.Fatalf("Description = %q", scope.Description)
	}
	if len(scope.Actions) != 2 {
		t.Fatalf("actions = %d, want 2", len(scope.Actions))
	}
	if len(scope.Goals) != 1 {
		t.Fatalf("goals = %d, want 1", len(scope.Goals))
	}
}

func TestBuildScopeAgent_DefaultDescription(t *testing.T) {
	scope := runtime.BuildScopeAgent(runtime.ScopeRun{
		Name:   "auto",
		Agents: []*core.Agent{scopeAgentA(), scopeAgentB()},
	})
	if !strings.Contains(scope.Description, "synthetic scope across 2 agent") {
		t.Fatalf("default description = %q", scope.Description)
	}
}
