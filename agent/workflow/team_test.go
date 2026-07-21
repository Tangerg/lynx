package workflow_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/agent/workflow"
)

type teamInput struct{ Text string }
type teamLength struct{ V int }
type teamResult struct{ Result int }

func teamAgentA() *core.Agent {
	return agent.New(agent.AgentConfig{Name: "team:A", Actions: []agent.Action{agent.NewAction("A:tokenize", func(_ context.Context, _ *core.ProcessContext, input teamInput) (teamLength, error) {
		return teamLength{V: len(input.Text)}, nil
	}, core.ActionConfig{})}})
}

func teamAgentB() *core.Agent {
	return agent.New(agent.AgentConfig{Name: "team:B", Actions: []agent.Action{agent.NewAction("B:double", func(_ context.Context, _ *core.ProcessContext, length teamLength) (teamResult, error) {
		return teamResult{Result: length.V * 2}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[teamResult](core.GoalConfig{Description: "joint output"})}})
}

func TestTeamRunsThroughOrdinaryEnginePath(t *testing.T) {
	definition, err := workflow.Team(workflow.TeamConfig{
		Name:   "team:joint",
		Agents: []*core.Agent{teamAgentA(), teamAgentB()},
	})
	if err != nil {
		t.Fatalf("Team: %v", err)
	}
	engine := agent.MustNewEngine(runtime.Config{})
	process, err := engine.Run(t.Context(), definition, core.Input(teamInput{Text: "lynx"}), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	result, ok := core.Result[teamResult](process)
	if !ok || result.Result != 8 {
		t.Fatalf("Result = %#v, %t; want 8, true", result, ok)
	}
	if history := process.History(); len(history) != 2 {
		t.Fatalf("history length = %d, want 2", len(history))
	}
}

func TestTeamDefinitionReusesOrdinaryDeployment(t *testing.T) {
	definition, err := workflow.Team(workflow.TeamConfig{
		Name:   "team:reuse",
		Agents: []*core.Agent{teamAgentA(), teamAgentB()},
	})
	if err != nil {
		t.Fatal(err)
	}
	engine := agent.MustNewEngine(runtime.Config{})
	for range 2 {
		if _, err := engine.Run(t.Context(), definition, core.Input(teamInput{Text: "ab"}), core.ProcessOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	deployment, ok := engine.ActiveDeployment("team:reuse")
	if !ok || deployment.Ref().Name != "team:reuse" {
		t.Fatalf("deployment = %#v, %t", deployment, ok)
	}
	if got := len(engine.ActiveDeployments()); got != 1 {
		t.Fatalf("active deployments = %d, want 1", got)
	}
}

func TestChangedTeamUsesOrdinaryDeploymentConflict(t *testing.T) {
	first, err := workflow.Team(workflow.TeamConfig{
		Name:   "team:conflict",
		Agents: []*core.Agent{teamAgentA(), teamAgentB()},
	})
	if err != nil {
		t.Fatal(err)
	}
	changed, err := workflow.Team(workflow.TeamConfig{
		Name:        "team:conflict",
		Description: "changed",
		Agents:      []*core.Agent{teamAgentA(), teamAgentB()},
	})
	if err != nil {
		t.Fatal(err)
	}
	engine := agent.MustNewEngine(runtime.Config{})
	if _, err := engine.Run(t.Context(), first, core.Input(teamInput{Text: "ab"}), core.ProcessOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Run(t.Context(), changed, core.Input(teamInput{Text: "ab"}), core.ProcessOptions{}); !errors.Is(err, runtime.ErrDeploymentConflict) {
		t.Fatalf("Run error = %v, want ErrDeploymentConflict", err)
	}
}

func TestTeamValidatesConfiguration(t *testing.T) {
	duplicate := func(name string) *core.Agent {
		return agent.New(agent.AgentConfig{Name: name, Actions: []agent.Action{agent.NewAction("same-name", func(context.Context, *core.ProcessContext, teamInput) (teamResult, error) {
			return teamResult{}, nil
		}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[teamResult](core.GoalConfig{Description: "ok"})}})
	}
	tests := []struct {
		name     string
		config   workflow.TeamConfig
		contains string
	}{
		{name: "empty name", config: workflow.TeamConfig{Agents: []*core.Agent{teamAgentA()}}, contains: "Name must not be empty"},
		{name: "empty agents", config: workflow.TeamConfig{Name: "empty"}, contains: "Agents must not be empty"},
		{name: "nil agent", config: workflow.TeamConfig{Name: "nil", Agents: []*core.Agent{teamAgentA(), nil}}, contains: "Agents[1] is nil"},
		{name: "duplicate action", config: workflow.TeamConfig{Name: "duplicate", Agents: []*core.Agent{duplicate("a"), duplicate("b")}}, contains: `duplicate action name "same-name"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			definition, err := workflow.Team(test.config)
			if definition != nil || err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Team = %#v, %v; want nil and %q", definition, err, test.contains)
			}
		})
	}
}

func TestTeamBuildsDefinition(t *testing.T) {
	definition, err := workflow.Team(workflow.TeamConfig{
		Name:        "inspect",
		Description: "custom",
		Agents:      []*core.Agent{teamAgentA(), teamAgentB()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if definition.Name() != "inspect" || definition.Description() != "custom" {
		t.Fatalf("definition = %q, %q", definition.Name(), definition.Description())
	}
	if len(definition.Actions()) != 2 || len(definition.Goals()) != 1 {
		t.Fatalf("actions = %d, goals = %d", len(definition.Actions()), len(definition.Goals()))
	}
}

func TestTeamPreservesMemberDurableState(t *testing.T) {
	first := teamAgentA()
	firstWithState := agent.New(agent.AgentConfig{
		Name:         first.Name(),
		Actions:      first.Actions(),
		DurableState: []core.Binding{core.NewBinding[int]("team_first_state")},
	})
	second := teamAgentB()
	secondWithState := agent.New(agent.AgentConfig{
		Name:         second.Name(),
		Actions:      second.Actions(),
		Goals:        second.Goals(),
		DurableState: []core.Binding{core.NewBinding[string]("team_second_state")},
	})

	definition, err := workflow.Team(workflow.TeamConfig{
		Name:   "team:durable",
		Agents: []*core.Agent{firstWithState, secondWithState},
	})
	if err != nil {
		t.Fatal(err)
	}
	state := definition.DurableState()
	if len(state) != 2 || state[0].Name != "team_first_state" || state[1].Name != "team_second_state" {
		t.Fatalf("durable state = %#v", state)
	}
}

func TestTeamRejectsDuplicateMemberDurableState(t *testing.T) {
	first := teamAgentA()
	firstWithState := agent.New(agent.AgentConfig{
		Name:         first.Name(),
		Actions:      first.Actions(),
		DurableState: []core.Binding{core.NewBinding[int]("shared_state")},
	})
	second := teamAgentB()
	secondWithState := agent.New(agent.AgentConfig{
		Name:         second.Name(),
		Actions:      second.Actions(),
		Goals:        second.Goals(),
		DurableState: []core.Binding{core.NewBinding[int]("shared_state")},
	})
	definition, err := workflow.Team(workflow.TeamConfig{
		Name:   "team:duplicate-durable",
		Agents: []*core.Agent{firstWithState, secondWithState},
	})
	if definition != nil || err == nil || !strings.Contains(err.Error(), "duplicate durable state") {
		t.Fatalf("Team = %#v, %v; want duplicate durable-state error", definition, err)
	}
}

func TestTeamBuildsDefaultDescription(t *testing.T) {
	definition, err := workflow.Team(workflow.TeamConfig{
		Name:   "auto",
		Agents: []*core.Agent{teamAgentA(), teamAgentB()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(definition.Description(), "synthetic team across 2 agents") {
		t.Fatalf("description = %q", definition.Description())
	}
}
