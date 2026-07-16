package runtime_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

type teamInput struct{ Text string }
type teamLength struct{ V int }
type teamResult struct{ Result int }

// agentA contributes the first half of the joint plan: word → teamLength.
func teamAgentA() *core.Agent {
	return agent.New(agent.AgentConfig{Name: "team:A", Actions: []agent.Action{agent.NewAction("A:tokenize", func(ctx context.Context, pc *core.ProcessContext, in teamInput) (teamLength, error) {
		return teamLength{V: len(in.Text)}, nil
	}, core.ActionConfig{})}})
}

// agentB contributes the second half: teamLength → teamResult.
func teamAgentB() *core.Agent {
	return agent.New(agent.AgentConfig{Name: "team:B", Actions: []agent.Action{agent.NewAction("B:double", func(ctx context.Context, pc *core.ProcessContext, in teamLength) (teamResult, error) {
		return teamResult{Result: in.V * 2}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[teamResult](core.GoalConfig{Description: "joint output"})}})
}

// TestRunTeam_CrossAgentPlanning is the headline case: two agents
// each carry one action and (in B's case) the goal. RunTeam unions
// them and the planner picks the path A:tokenize → B:double.
func TestRunTeam_CrossAgentPlanning(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})

	proc, err := engine.RunTeam(
		context.Background(),
		runtime.TeamConfig{
			Name:   "team:joint",
			Agents: []*core.Agent{teamAgentA(), teamAgentB()},
		},
		map[string]any{core.DefaultBindingName: teamInput{Text: "lynx"}},
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("RunTeam: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %s, failure = %v", proc.Status(), proc.Failure())
	}
	got, ok := core.Result[teamResult](proc)
	if !ok {
		t.Fatalf("no teamResult in result")
	}
	if got.Result != 8 { // len("lynx") * 2
		t.Fatalf("Result = %d, want 8", got.Result)
	}
	if len(proc.History()) != 2 {
		t.Fatalf("history length = %d, want 2 (one action from each agent)", len(proc.History()))
	}
}

func TestRunTeam_ReusesExactSyntheticDeployment(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})

	config := runtime.TeamConfig{
		Name:   "team:reuse",
		Agents: []*core.Agent{teamAgentA(), teamAgentB()},
	}
	for i := range 2 {
		if _, err := engine.RunTeam(
			context.Background(), config,
			map[string]any{core.DefaultBindingName: teamInput{Text: "ab"}},
			core.ProcessOptions{},
		); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	deployment, ok := engine.ActiveDeployment("team:reuse")
	if !ok {
		t.Fatal("team agent not registered after RunTeam")
	}
	if deployment.Ref().Name != "team:reuse" {
		t.Fatalf("team agent name = %q", deployment.Ref().Name)
	}
	if len(engine.ActiveDeployments()) != 1 {
		t.Fatalf("active deployments = %d, want 1", len(engine.ActiveDeployments()))
	}
}

func TestRunTeam_RejectsChangedDefinitionUnderActiveName(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	first := runtime.TeamConfig{Name: "team:conflict", Agents: []*core.Agent{teamAgentA(), teamAgentB()}}
	if _, err := engine.RunTeam(
		context.Background(), first,
		map[string]any{core.DefaultBindingName: teamInput{Text: "ab"}},
		core.ProcessOptions{},
	); err != nil {
		t.Fatal(err)
	}

	changed := first
	changed.Description = "changed"
	_, err := engine.RunTeam(
		context.Background(), changed,
		map[string]any{core.DefaultBindingName: teamInput{Text: "ab"}},
		core.ProcessOptions{},
	)
	if !errors.Is(err, runtime.ErrDeploymentConflict) {
		t.Fatalf("error = %v, want ErrDeploymentConflict", err)
	}
}

func TestRunTeam_EmptyName(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	_, err := engine.RunTeam(
		context.Background(),
		runtime.TeamConfig{Agents: []*core.Agent{teamAgentA()}},
		nil, core.ProcessOptions{},
	)
	if err == nil || !strings.Contains(err.Error(), "name must not be empty") {
		t.Fatalf("want Name-empty error, got %v", err)
	}
}

func TestRunTeam_EmptyAgents(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	_, err := engine.RunTeam(
		context.Background(),
		runtime.TeamConfig{Name: "empty"},
		nil, core.ProcessOptions{},
	)
	if err == nil || !strings.Contains(err.Error(), "agents must not be empty") {
		t.Fatalf("want Agents-empty error, got %v", err)
	}
}

func TestRunTeam_NilAgentEntry(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	_, err := engine.RunTeam(
		context.Background(),
		runtime.TeamConfig{
			Name:   "with-nil",
			Agents: []*core.Agent{teamAgentA(), nil},
		},
		nil, core.ProcessOptions{},
	)
	if err == nil || !strings.Contains(err.Error(), "agents[1] is nil") {
		t.Fatalf("want nil-entry error, got %v", err)
	}
}

// TestRunTeam_DuplicateActionNameAcrossAgents verifies that
// collisions across the union are rejected by Deploy (the synthetic
// agent must satisfy core.Validate). The user is responsible for
// prefixing names per agent to avoid this.
func TestRunTeam_DuplicateActionNameAcrossAgents(t *testing.T) {
	dup := func(name string) *core.Agent {
		return agent.New(agent.AgentConfig{Name: name, Actions: []agent.Action{agent.NewAction("same-name", func(ctx context.Context, pc *core.ProcessContext, in teamInput) (teamResult, error) {
			return teamResult{}, nil
		}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[teamResult](core.GoalConfig{Description: "ok"})}})
	}

	engine := agent.MustNewEngine(runtime.Config{})
	_, err := engine.RunTeam(
		context.Background(),
		runtime.TeamConfig{
			Name:   "team:dup",
			Agents: []*core.Agent{dup("agent-1"), dup("agent-2")},
		},
		nil, core.ProcessOptions{},
	)
	if err == nil || !strings.Contains(err.Error(), `duplicate action name "same-name"`) {
		t.Fatalf("want duplicate-name error, got %v", err)
	}
}

func TestRunTeamBuildsSyntheticDefinition(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	_, err := engine.RunTeam(t.Context(), runtime.TeamConfig{
		Name:        "inspect",
		Description: "custom",
		Agents:      []*core.Agent{teamAgentA(), teamAgentB()},
	}, map[string]any{core.DefaultBindingName: teamInput{Text: "lynx"}}, core.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	deployment, ok := engine.ActiveDeployment("inspect")
	if !ok {
		t.Fatal("synthetic deployment not found")
	}
	team := deployment.Agent()
	if team.Name() != "inspect" {
		t.Fatalf("Name = %q", team.Name())
	}
	if team.Description() != "custom" {
		t.Fatalf("Description = %q", team.Description())
	}
	if len(team.Actions()) != 2 {
		t.Fatalf("actions = %d, want 2", len(team.Actions()))
	}
	if len(team.Goals()) != 1 {
		t.Fatalf("goals = %d, want 1", len(team.Goals()))
	}
}

func TestRunTeamBuildsDefaultDescription(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	_, err := engine.RunTeam(t.Context(), runtime.TeamConfig{
		Name:   "auto",
		Agents: []*core.Agent{teamAgentA(), teamAgentB()},
	}, map[string]any{core.DefaultBindingName: teamInput{Text: "lynx"}}, core.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	deployment, ok := engine.ActiveDeployment("auto")
	if !ok {
		t.Fatal("synthetic deployment not found")
	}
	team := deployment.Agent()
	if !strings.Contains(team.Description(), "synthetic team across 2 agent") {
		t.Fatalf("default description = %q", team.Description())
	}
}
