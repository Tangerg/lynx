package planning_test

import (
	"context"
	"errors"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning"
)

type domainInput struct{ X int }
type domainOutput struct{ Y int }

type planAction struct{ metadata core.ActionMetadata }

func (a *planAction) Metadata() core.ActionMetadata { return a.metadata }
func (*planAction) Execute(context.Context, *core.ProcessContext) (core.ActionStatus, error) {
	return core.ActionSucceeded, nil
}

type plannerFunc func(*core.Goal) *planning.Plan

func (plannerFunc) Name() string { return "test-planner" }
func (f plannerFunc) PlanToGoal(_ context.Context, _ core.WorldState, _ *planning.Domain, goal *core.Goal, _ planning.Options) (*planning.Plan, error) {
	return f(goal), nil
}

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

func TestDomainRejectsInvalidPlannerResults(t *testing.T) {
	canonical := &planAction{metadata: core.ActionMetadata{
		Name:    "work",
		Effects: core.ConditionSet{"done": core.True},
	}}
	blocked := &planAction{metadata: core.ActionMetadata{
		Name:          "blocked",
		Preconditions: core.ConditionSet{"ready": core.True},
		Effects:       core.ConditionSet{"done": core.True},
	}}
	noop := &planAction{metadata: core.ActionMetadata{Name: "noop"}}
	goal := core.NewGoal(core.GoalConfig{Name: "goal", Preconditions: []string{"done"}})
	otherGoal := core.NewGoal(core.GoalConfig{Name: "other", Preconditions: []string{"done"}})
	domain := mustDomain(t, []core.Action{canonical, blocked, noop}, []*core.Goal{goal}, nil)
	state := planning.NewState(nil)

	tests := []struct {
		name    string
		plan    func(*core.Goal) *planning.Plan
		options planning.Options
	}{
		{"different goal", func(*core.Goal) *planning.Plan { return planning.NewPlan(nil, otherGoal) }, planning.Options{}},
		{"nil action", func(goal *core.Goal) *planning.Plan { return planning.NewPlan([]core.Action{nil}, goal) }, planning.Options{}},
		{"outside action", func(goal *core.Goal) *planning.Plan {
			return planning.NewPlan([]core.Action{&planAction{metadata: core.ActionMetadata{Name: "rogue"}}}, goal)
		}, planning.Options{}},
		{"excluded action", func(goal *core.Goal) *planning.Plan { return planning.NewPlan([]core.Action{canonical}, goal) }, planning.Options{ExcludedActions: planning.NewExclusions("work")}},
		{"unsatisfied preconditions", func(goal *core.Goal) *planning.Plan { return planning.NewPlan([]core.Action{blocked}, goal) }, planning.Options{}},
		{"goal not achieved", func(goal *core.Goal) *planning.Plan {
			return planning.NewPlan([]core.Action{noop}, goal)
		}, planning.Options{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := domain.Plans(t.Context(), plannerFunc(test.plan), state, test.options)
			if err == nil || !strings.Contains(err.Error(), "planning: invalid plan") {
				t.Fatalf("Plans() error = %v, want invalid plan", err)
			}
		})
	}
}

func TestDomainCanonicalizesPlannerActions(t *testing.T) {
	canonical := &planAction{metadata: core.ActionMetadata{Name: "work", Effects: core.ConditionSet{"done": core.True}}}
	lookalike := &planAction{metadata: canonical.metadata}
	goal := core.NewGoal(core.GoalConfig{Name: "goal", Preconditions: []string{"done"}})
	domain := mustDomain(t, []core.Action{canonical}, []*core.Goal{goal}, nil)

	plans, err := domain.Plans(t.Context(), plannerFunc(func(goal *core.Goal) *planning.Plan {
		return planning.NewPlan([]core.Action{lookalike}, goal)
	}), planning.NewState(nil), planning.Options{})
	if err != nil {
		t.Fatalf("Plans: %v", err)
	}
	if got := plans[0].Actions()[0]; got != canonical {
		t.Fatalf("accepted action = %p, want canonical %p", got, canonical)
	}
}

func TestDomainRejectsInvalidPlanScores(t *testing.T) {
	panicCause := errors.New("score sentinel")
	tests := []struct {
		name      string
		cost      core.ScoreFunc
		value     core.ScoreFunc
		goalValue core.ScoreFunc
		contains  string
		cause     error
	}{
		{name: "negative cost", cost: core.FixedScore(-1), contains: "cost must be finite and non-negative"},
		{name: "infinite action value", value: core.FixedScore(math.Inf(1)), contains: `action "work" value returned +Inf`},
		{name: "nan goal value", goalValue: core.FixedScore(math.NaN()), contains: `goal "goal" value returned NaN`},
		{name: "panicked cost", cost: func(core.WorldState) float64 { panic(panicCause) }, contains: "score function panicked", cause: panicCause},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			action := &planAction{metadata: core.ActionMetadata{
				Name: "work", Effects: core.ConditionSet{"done": core.True}, Cost: test.cost, Value: test.value,
			}}
			goal := core.NewGoal(core.GoalConfig{Name: "goal", Preconditions: []string{"done"}, Value: test.goalValue})
			domain := mustDomain(t, []core.Action{action}, []*core.Goal{goal}, nil)
			_, err := domain.Plans(t.Context(), plannerFunc(func(goal *core.Goal) *planning.Plan {
				return planning.NewPlan([]core.Action{action}, goal)
			}), planning.NewState(nil), planning.Options{})
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Plans error = %v, want %q", err, test.contains)
			}
			if test.cause != nil && !errors.Is(err, test.cause) {
				t.Fatalf("Plans error = %v, want cause %v", err, test.cause)
			}
		})
	}
}
