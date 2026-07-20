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

func mustDomain(t *testing.T, actions []core.Action, goals []*core.Goal, conditions []core.Condition) *planning.Domain {
	t.Helper()
	domain, err := planning.NewDomain(actions, goals, conditions)
	if err != nil {
		t.Fatalf("NewDomain: %v", err)
	}
	return domain
}

func mustDomainForAgents(t *testing.T, agents []*core.Agent) *planning.Domain {
	t.Helper()
	domain, err := planning.DomainForAgents(agents)
	if err != nil {
		t.Fatalf("DomainForAgents: %v", err)
	}
	return domain
}

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

	domain := mustDomainForAgents(t, []*core.Agent{alpha, beta})
	if len(domain.Actions()) != 2 {
		t.Fatalf("actions = %d, want 2", len(domain.Actions()))
	}
	if len(domain.Goals()) != 2 {
		t.Fatalf("goals = %d, want 2", len(domain.Goals()))
	}
}

func TestDomainForAgentsSkipsNilEntries(t *testing.T) {
	domain := mustDomainForAgents(t, []*core.Agent{nil, domainAgent("only", "only:step"), nil})
	if len(domain.Actions()) != 1 {
		t.Fatalf("actions = %d, want 1", len(domain.Actions()))
	}
}

func TestDomainForAgentsEmptyInputProducesEmptyDomain(t *testing.T) {
	domain := mustDomainForAgents(t, nil)
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
	domain := mustDomain(t, actions, goals, nil)

	actions[0] = nil
	goals[0] = nil
	if domain.Actions()[0] == nil || domain.Goals()[0] == nil {
		t.Fatal("NewDomain retained caller-owned slice storage")
	}

	refs := slices.Collect(domain.KnownConditions())
	conditions := make([]string, len(refs))
	for index, ref := range refs {
		conditions[index] = ref.Key
	}
	if !slices.Equal(conditions[len(conditions)-2:], []string{"a_done", "z_done"}) {
		t.Fatalf("KnownConditions tail = %v, want deterministic goal keys", conditions)
	}
}

func TestNewDomainPreservesConditionSourcesWithoutParsingKeys(t *testing.T) {
	action := domainAgent("worker", "worker:step").Actions()[0]
	conditions := []core.Condition{
		core.NewCondition("external:ready", nil),
		core.NewCondition("action_ran_external", nil),
	}
	domain := mustDomain(t, []core.Action{action}, nil, conditions)

	want := map[string]planning.ConditionKind{
		action.Metadata().RunCondition(): planning.ConditionActionRun,
		"external:ready":                 planning.ConditionEvaluator,
		"action_ran_external":            planning.ConditionEvaluator,
	}
	for ref := range domain.KnownConditions() {
		if kind, ok := want[ref.Key]; ok {
			if ref.Kind != kind {
				t.Errorf("condition %q kind = %v, want %v", ref.Key, ref.Kind, kind)
			}
			delete(want, ref.Key)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing condition refs: %v", want)
	}
}

func TestNewDomainRejectsConflictingConditionSources(t *testing.T) {
	action := domainAgent("worker", "checkout").Actions()[0]
	_, err := planning.NewDomain(
		[]core.Action{action},
		nil,
		[]core.Condition{core.NewCondition(action.Metadata().RunCondition(), nil)},
	)
	if err == nil {
		t.Fatal("NewDomain accepted an evaluator that shadows an action-run condition")
	}
}

func TestDomainPlanningMethodsValidateTheirInputs(t *testing.T) {
	domain := mustDomain(t, nil, nil, nil)
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
