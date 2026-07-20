package planning_test

import (
	"context"
	"slices"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning"
)

type domainInput struct{ X int }
type domainOutput struct{ Y int }

func TestEffectivePlannerName(t *testing.T) {
	if got := planning.EffectivePlannerName(""); got != planning.GOAPPlannerName {
		t.Fatalf("EffectivePlannerName(empty) = %q, want %q", got, planning.GOAPPlannerName)
	}
	if got := planning.EffectivePlannerName(planning.HTNPlannerName); got != planning.HTNPlannerName {
		t.Fatalf("EffectivePlannerName(htn) = %q", got)
	}
}

func TestExclusionsAreZeroValueUsableAndImmutable(t *testing.T) {
	var empty planning.Exclusions
	withA := empty.With("a")
	withAB := withA.With("b")

	if empty.Contains("a") || !withA.Contains("a") || withA.Contains("b") {
		t.Fatal("With mutated an existing exclusion set")
	}
	if !withAB.Contains("a") || !withAB.Contains("b") {
		t.Fatal("With did not preserve existing exclusions")
	}
}

func domainAgent(name, actionName string) *core.Agent {
	return agent.New(agent.AgentConfig{Name: name, Actions: []agent.Action{agent.NewAction(actionName, func(_ context.Context, _ *core.ProcessContext, input domainInput) (domainOutput, error) {
		return domainOutput{Y: input.X + 1}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[domainOutput](core.GoalConfig{Description: name + " goal"})}})
}

func TestDomainForAgentsUnionsCapabilities(t *testing.T) {
	alpha := domainAgent("alpha", "alpha:step")
	beta := domainAgent("beta", "beta:step")

	domain := planning.DomainForAgents([]*core.Agent{alpha, beta})
	if len(domain.Actions()) != 2 {
		t.Fatalf("actions = %d, want 2", len(domain.Actions()))
	}
	if len(domain.Goals()) != 2 {
		t.Fatalf("goals = %d, want 2", len(domain.Goals()))
	}
}

func TestDomainForAgentsSkipsNilEntries(t *testing.T) {
	domain := planning.DomainForAgents([]*core.Agent{nil, domainAgent("only", "only:step"), nil})
	if len(domain.Actions()) != 1 {
		t.Fatalf("actions = %d, want 1", len(domain.Actions()))
	}
}

func TestDomainForAgentsEmptyInputProducesEmptyDomain(t *testing.T) {
	domain := planning.DomainForAgents(nil)
	if domain == nil {
		t.Fatal("domain is nil")
	}
	if len(domain.Actions()) != 0 || len(domain.Goals()) != 0 {
		t.Fatalf("non-empty domain: actions=%d goals=%d", len(domain.Actions()), len(domain.Goals()))
	}
}

func TestNewDomainCopiesInputsAndOrdersKnownConditions(t *testing.T) {
	action := domainAgent("copy", "copy:step").Actions()[0]
	actions := []core.Action{action}
	goals := []*core.Goal{core.NewGoal(core.GoalConfig{Name: "goal", Preconditions: []string{"z_done", "a_done"}})}
	domain := planning.NewDomain(actions, goals, nil)

	actions[0] = nil
	goals[0] = nil
	if domain.Actions()[0] == nil || domain.Goals()[0] == nil {
		t.Fatal("NewDomain retained caller-owned slice storage")
	}

	conditions := slices.Collect(domain.KnownConditions())
	if !slices.Equal(conditions[len(conditions)-2:], []string{"a_done", "z_done"}) {
		t.Fatalf("KnownConditions tail = %v, want deterministic goal keys", conditions)
	}
}

func TestDomainPlanningMethodsValidateTheirInputs(t *testing.T) {
	domain := planning.NewDomain(nil, nil, nil)
	state := planning.NewState(nil)
	goal := core.NewGoal(core.GoalConfig{Name: "goal"})

	if err := domain.ValidatePlanInputs(state, goal); err != nil {
		t.Fatalf("ValidatePlanInputs: %v", err)
	}
	if _, err := domain.Plans(t.Context(), nil, state, planning.Options{}); err == nil {
		t.Fatal("Plans accepted nil planner")
	}

	var nilDomain *planning.Domain
	if err := nilDomain.ValidatePlanInputs(state, goal); err == nil {
		t.Fatal("ValidatePlanInputs accepted nil domain")
	}
}
