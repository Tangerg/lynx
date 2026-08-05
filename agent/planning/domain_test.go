package planning_test

import (
	"context"
	"errors"
	"maps"
	"math"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning"
)

type domainInput struct{ X int }
type domainOutput struct{ Y int }

type planAction struct {
	metadata core.ActionMetadata
	runs     int
}

func (a *planAction) Metadata() core.ActionMetadata { return a.metadata }
func (a *planAction) Execute(context.Context, *core.ProcessContext) (core.ActionStatus, error) {
	a.runs++
	return core.ActionSucceeded, nil
}

type planCondition struct {
	name           string
	evaluationCost float64
	evaluations    int
}

func (c *planCondition) Name() string            { return c.name }
func (c *planCondition) EvaluationCost() float64 { return c.evaluationCost }
func (c *planCondition) Evaluate(context.Context, *core.ConditionEnv) core.Truth {
	c.evaluations++
	return core.True
}

type panickingDomainAction struct{ cause error }

func (a panickingDomainAction) Metadata() core.ActionMetadata { panic(a.cause) }
func (panickingDomainAction) Execute(context.Context, *core.ProcessContext) (core.ActionStatus, error) {
	return core.ActionSucceeded, nil
}

type panickingDomainCondition struct {
	nameCause           error
	evaluationCostCause error
}

func (c panickingDomainCondition) Name() string {
	if c.nameCause != nil {
		panic(c.nameCause)
	}
	return "condition"
}
func (c panickingDomainCondition) EvaluationCost() float64 {
	if c.evaluationCostCause != nil {
		panic(c.evaluationCostCause)
	}
	return 0
}
func (panickingDomainCondition) Evaluate(context.Context, *core.ConditionEnv) core.Truth {
	return core.True
}

type plannerFunc func(*core.Goal) *planning.Plan

func (plannerFunc) Name() string { return "test-planner" }
func (f plannerFunc) PlanToGoal(_ context.Context, _ core.WorldState, _ *planning.Domain, goal *core.Goal, _ planning.Options) (*planning.Plan, error) {
	return f(goal), nil
}

type conditionResolverFunc struct {
	mu       sync.Mutex
	resolve  func(context.Context, string) (core.Truth, error)
	resolved core.ConditionSet
}

type panickingConditionResolver struct {
	resolveCause  error
	snapshotCause error
}

func (r panickingConditionResolver) Resolve(context.Context, string) (core.Truth, error) {
	panic(r.resolveCause)
}

func (r panickingConditionResolver) ResolvedConditions() core.ConditionSet {
	panic(r.snapshotCause)
}

func newConditionResolver(resolve func(context.Context, string) (core.Truth, error)) *conditionResolverFunc {
	return &conditionResolverFunc{resolve: resolve, resolved: make(core.ConditionSet)}
}

func (r *conditionResolverFunc) Resolve(ctx context.Context, name string) (core.Truth, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	truth, err := r.resolve(ctx, name)
	if err == nil {
		r.resolved[name] = truth
	}
	return truth, err
}

func (r *conditionResolverFunc) ResolvedConditions() core.ConditionSet {
	r.mu.Lock()
	defer r.mu.Unlock()
	return maps.Clone(r.resolved)
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

func TestDomainSatisfiesResolvesUnknownConditionsByEvaluationCost(t *testing.T) {
	domain := mustDomain(t, nil, nil, []core.Condition{
		core.NewCondition(core.ConditionConfig{Name: "expensive", EvaluationCost: 100}),
		core.NewCondition(core.ConditionConfig{Name: "cheap", EvaluationCost: 1}),
	})
	state := planning.NewState(core.ConditionSet{"cheap": core.Unknown, "expensive": core.Unknown})
	var resolved []string
	resolver := newConditionResolver(func(_ context.Context, name string) (core.Truth, error) {
		resolved = append(resolved, name)
		if name == "cheap" {
			return core.False, nil
		}
		return core.True, nil
	})

	satisfied, err := domain.Satisfies(
		t.Context(),
		state,
		core.ConditionSet{"cheap": core.True, "expensive": core.True},
		resolver,
	)
	if err != nil {
		t.Fatalf("Satisfies: %v", err)
	}
	if satisfied {
		t.Fatal("Satisfies = true, want cheap mismatch")
	}
	if !slices.Equal(resolved, []string{"cheap"}) {
		t.Fatalf("resolved = %v, want only cheap", resolved)
	}
}

func TestDomainSatisfiesResolvesMultipleUnknownConditionsByEvaluationCost(t *testing.T) {
	domain := mustDomain(t, nil, nil, []core.Condition{
		core.NewCondition(core.ConditionConfig{Name: "third", EvaluationCost: 3}),
		core.NewCondition(core.ConditionConfig{Name: "first", EvaluationCost: 1}),
		core.NewCondition(core.ConditionConfig{Name: "second", EvaluationCost: 2}),
	})
	var resolved []string
	resolver := newConditionResolver(func(_ context.Context, name string) (core.Truth, error) {
		resolved = append(resolved, name)
		return core.True, nil
	})

	satisfied, err := domain.Satisfies(
		t.Context(),
		planning.NewState(nil),
		core.ConditionSet{"first": core.True, "second": core.True, "third": core.True},
		resolver,
	)
	if err != nil {
		t.Fatalf("Satisfies: %v", err)
	}
	if !satisfied {
		t.Fatal("Satisfies = false, want true")
	}
	if !slices.Equal(resolved, []string{"first", "second", "third"}) {
		t.Fatalf("resolved = %v, want cost order", resolved)
	}
}

func TestDomainSatisfiesSkipsEvaluationAfterKnownMismatch(t *testing.T) {
	domain := mustDomain(t, nil, nil, []core.Condition{
		core.NewCondition(core.ConditionConfig{Name: "remote", EvaluationCost: 1}),
	})
	resolver := newConditionResolver(func(context.Context, string) (core.Truth, error) {
		t.Fatal("resolver called after a known mismatch")
		return core.Unknown, nil
	})

	satisfied, err := domain.Satisfies(
		t.Context(),
		planning.NewState(core.ConditionSet{"local": core.False}),
		core.ConditionSet{"local": core.True, "remote": core.True},
		resolver,
	)
	if err != nil {
		t.Fatalf("Satisfies: %v", err)
	}
	if satisfied {
		t.Fatal("Satisfies = true, want known mismatch")
	}
}

func TestDomainApplicableActionsShortCircuitsExpensiveConditions(t *testing.T) {
	blocked := &planAction{metadata: core.ActionMetadata{
		Name:          "blocked",
		Preconditions: core.ConditionSet{"cheap": core.True, "expensive": core.True},
	}}
	ready := &planAction{metadata: core.ActionMetadata{Name: "ready"}}
	domain := mustDomain(t, []core.Action{blocked, ready}, nil, []core.Condition{
		core.NewCondition(core.ConditionConfig{Name: "expensive", EvaluationCost: 100}),
		core.NewCondition(core.ConditionConfig{Name: "cheap", EvaluationCost: 1}),
	})
	var resolved []string
	resolver := newConditionResolver(func(_ context.Context, name string) (core.Truth, error) {
		resolved = append(resolved, name)
		if name == "expensive" {
			t.Fatal("expensive condition evaluated after cheap condition rejected its action")
		}
		return core.False, nil
	})

	applicable, err := domain.ApplicableActions(t.Context(), planning.NewState(nil), domain.Actions(), resolver)
	if err != nil {
		t.Fatalf("ApplicableActions: %v", err)
	}
	if len(applicable) != 1 || applicable[0].Metadata().Name != "ready" {
		t.Fatalf("applicable = %v, want ready", actionNames(applicable))
	}
	if !slices.Equal(resolved, []string{"cheap"}) {
		t.Fatalf("resolved = %v, want only cheap", resolved)
	}
}

func TestDomainApplicableActionsTreatsObservedUnknownAsMismatch(t *testing.T) {
	action := &planAction{metadata: core.ActionMetadata{
		Name:          "guarded",
		Preconditions: core.ConditionSet{"remote": core.True},
	}}
	domain := mustDomain(t, []core.Action{action}, nil, []core.Condition{
		core.NewCondition(core.ConditionConfig{Name: "remote", EvaluationCost: 1}),
	})
	calls := 0
	resolver := newConditionResolver(func(context.Context, string) (core.Truth, error) {
		calls++
		return core.Unknown, nil
	})

	applicable, err := domain.ApplicableActions(t.Context(), planning.NewState(nil), domain.Actions(), resolver)
	if err != nil {
		t.Fatalf("ApplicableActions: %v", err)
	}
	if len(applicable) != 0 || calls != 1 {
		t.Fatalf("applicable/calls = %v/%d, want none/one", actionNames(applicable), calls)
	}
}

func TestDomainApplicableActionsRejectsActionsOutsideDomain(t *testing.T) {
	domain := mustDomain(t, nil, nil, nil)
	outside := &planAction{metadata: core.ActionMetadata{Name: "outside"}}

	_, err := domain.ApplicableActions(t.Context(), planning.NewState(nil), []core.Action{outside}, nil)
	if err == nil || !strings.Contains(err.Error(), `action[0] "outside" is outside the domain`) {
		t.Fatalf("ApplicableActions error = %v, want outside-domain rejection", err)
	}
}

func TestStateWithResolvedConditionsOverlaysObservationsWithoutOverwritingEffects(t *testing.T) {
	domain := mustDomain(t, nil, nil, []core.Condition{
		core.NewCondition(core.ConditionConfig{Name: "ready", EvaluationCost: 1}),
	})
	resolver := newConditionResolver(func(context.Context, string) (core.Truth, error) {
		return core.True, nil
	})
	if _, err := resolver.Resolve(t.Context(), "ready"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	base := planning.NewState(core.ConditionSet{"ready": core.Unknown})
	resolved, err := domain.StateWithResolvedConditions(base, resolver)
	if err != nil {
		t.Fatalf("StateWithResolvedConditions: %v", err)
	}
	if resolved.Conditions()["ready"] != core.True || base.Conditions()["ready"] != core.Unknown {
		t.Fatalf("resolved/base = %v/%v, want true/unknown", resolved.Conditions(), base.Conditions())
	}

	withEffect := base.Apply(core.ConditionSet{"ready": core.False})
	resolved, err = domain.StateWithResolvedConditions(withEffect, resolver)
	if err != nil {
		t.Fatalf("StateWithResolvedConditions with effect: %v", err)
	}
	if resolved.Conditions()["ready"] != core.False {
		t.Fatalf("resolved effect = %v, want false", resolved.Conditions())
	}
}

func TestDomainContainsConditionResolverPanics(t *testing.T) {
	domain := mustDomain(t, nil, nil, []core.Condition{
		core.NewCondition(core.ConditionConfig{Name: "ready", EvaluationCost: 1}),
	})
	resolveCause := errors.New("resolve sentinel")
	_, err := domain.Satisfies(
		t.Context(),
		planning.NewState(nil),
		core.ConditionSet{"ready": core.True},
		panickingConditionResolver{resolveCause: resolveCause},
	)
	if !errors.Is(err, resolveCause) || !strings.Contains(err.Error(), "panicked resolving") {
		t.Fatalf("Satisfies error = %v, want contained resolver panic", err)
	}

	snapshotCause := errors.New("snapshot sentinel")
	_, err = domain.StateWithResolvedConditions(
		planning.NewState(nil),
		panickingConditionResolver{snapshotCause: snapshotCause},
	)
	if !errors.Is(err, snapshotCause) || !strings.Contains(err.Error(), "ResolvedConditions panicked") {
		t.Fatalf("StateWithResolvedConditions error = %v, want contained snapshot panic", err)
	}
}

func TestDomainPlansContainsPlannerPanic(t *testing.T) {
	cause := errors.New("planner sentinel")
	definition := domainAgent("panic-domain", "step")
	domain := mustDomain(t, definition.Actions(), definition.Goals(), nil)
	planner := plannerFunc(func(*core.Goal) *planning.Plan { panic(cause) })

	_, err := domain.Plans(t.Context(), planner, planning.NewState(nil), planning.Options{})
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), `planner "test-planner"`) {
		t.Fatalf("Plans error = %v, want attributed planner panic", err)
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
	return agent.New(agent.Config{Name: name, Actions: []agent.Action{agent.NewAction(actionName, func(_ context.Context, _ *core.ProcessContext, input domainInput) (domainOutput, error) {
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

func TestNewDomainCopiesInputsAndOrdersConditionRefs(t *testing.T) {
	action := domainAgent("copy", "copy:step").Actions()[0]
	actions := []core.Action{action}
	goals := []*core.Goal{core.NewGoal(core.GoalConfig{Name: "goal", RequiredConditions: []string{"z_done", "a_done"}})}
	domain := mustDomain(t, actions, goals, nil)

	actions[0] = nil
	goals[0] = nil
	if domain.Actions()[0] == nil || domain.Goals()[0] == nil {
		t.Fatal("NewDomain retained caller-owned slice storage")
	}

	refs := slices.Collect(domain.ConditionRefs())
	conditions := make([]string, len(refs))
	for index, ref := range refs {
		conditions[index] = ref.Key
	}
	if !slices.Equal(conditions[len(conditions)-2:], []string{"a_done", "z_done"}) {
		t.Fatalf("ConditionRefs tail = %v, want deterministic goal keys", conditions)
	}
}

func TestNewDomainFreezesMetadataAndPreservesExecutionDelegates(t *testing.T) {
	action := &planAction{metadata: core.ActionMetadata{
		Name:          "work",
		Preconditions: core.ConditionSet{"ready": core.True},
		Effects:       core.ConditionSet{"done": core.True},
		ToolRoles:     []string{"workspace"},
	}}
	condition := &planCondition{name: "ready", evaluationCost: 2}
	domain := mustDomain(t, []core.Action{action}, nil, []core.Condition{condition})

	action.metadata.Name = "mutated"
	action.metadata.Preconditions["ready"] = core.False
	action.metadata.Effects["done"] = core.False
	action.metadata.ToolRoles[0] = "mutated"
	condition.name = "mutated"
	condition.evaluationCost = 99

	metadata := domain.Actions()[0].Metadata()
	if metadata.Name != "work" || metadata.Preconditions["ready"] != core.True ||
		metadata.Effects["done"] != core.True || !slices.Equal(metadata.ToolRoles, []string{"workspace"}) {
		t.Fatalf("frozen action metadata = %#v", metadata)
	}
	if got := domain.Conditions()[0]; got.Name() != "ready" || got.EvaluationCost() != 2 {
		t.Fatalf("frozen condition metadata = %q/%v", got.Name(), got.EvaluationCost())
	}

	metadata.Effects["done"] = core.False
	if domain.Actions()[0].Metadata().Effects["done"] != core.True {
		t.Fatal("Actions returned mutable domain metadata")
	}
	if _, err := domain.Actions()[0].Execute(t.Context(), nil); err != nil || action.runs != 1 {
		t.Fatalf("action delegate runs/error = %d/%v", action.runs, err)
	}
	if truth := domain.Conditions()[0].Evaluate(t.Context(), nil); truth != core.True || condition.evaluations != 1 {
		t.Fatalf("condition delegate truth/evaluations = %v/%d", truth, condition.evaluations)
	}
}

func TestNewDomainContainsMetadataPanics(t *testing.T) {
	actionCause := errors.New("action sentinel")
	nameCause := errors.New("name sentinel")
	evaluationCostCause := errors.New("evaluation-cost sentinel")
	for _, test := range []struct {
		name       string
		actions    []core.Action
		conditions []core.Condition
		cause      error
		contains   string
	}{
		{
			name: "action metadata", actions: []core.Action{panickingDomainAction{cause: actionCause}},
			cause: actionCause, contains: "Action.Metadata panicked",
		},
		{
			name: "condition name", conditions: []core.Condition{panickingDomainCondition{nameCause: nameCause}},
			cause: nameCause, contains: "Condition.Name panicked",
		},
		{
			name: "condition evaluation cost", conditions: []core.Condition{panickingDomainCondition{evaluationCostCause: evaluationCostCause}},
			cause: evaluationCostCause, contains: "Condition.EvaluationCost panicked",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := planning.NewDomain(test.actions, nil, test.conditions)
			if !errors.Is(err, test.cause) || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("NewDomain error = %v, want %q", err, test.contains)
			}
		})
	}
}

func TestNewDomainRejectsInvalidConditionCost(t *testing.T) {
	for _, cost := range []float64{-1, math.NaN(), math.Inf(1)} {
		_, err := planning.NewDomain(nil, nil, []core.Condition{&planCondition{name: "condition", evaluationCost: cost}})
		if err == nil || !strings.Contains(err.Error(), "must be finite and non-negative") {
			t.Fatalf("NewDomain cost %v error = %v", cost, err)
		}
	}
}

func TestNewDomainPreservesConditionSourcesWithoutParsingKeys(t *testing.T) {
	action := domainAgent("worker", "worker:step").Actions()[0]
	conditions := []core.Condition{
		core.NewCondition(core.ConditionConfig{Name: "external:ready"}),
		core.NewCondition(core.ConditionConfig{Name: "action_succeeded_external"}),
	}
	domain := mustDomain(t, []core.Action{action}, nil, conditions)

	want := map[string]planning.ConditionSourceKind{
		action.Metadata().SuccessCondition(): planning.ConditionActionSuccess,
		"external:ready":                     planning.ConditionEvaluator,
		"action_succeeded_external":          planning.ConditionEvaluator,
	}
	for ref := range domain.ConditionRefs() {
		if source, ok := want[ref.Key]; ok {
			if ref.Source != source {
				t.Errorf("condition %q source = %v, want %v", ref.Key, ref.Source, source)
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
		[]core.Condition{core.NewCondition(core.ConditionConfig{Name: action.Metadata().SuccessCondition()})},
	)
	if err == nil {
		t.Fatal("NewDomain accepted an evaluator that shadows an action-success condition")
	}
}

func TestNewDomainRejectsNilMembers(t *testing.T) {
	var typedNilAction *planAction
	var typedNilCondition *core.FuncCondition
	goal := core.NewGoal(core.GoalConfig{Name: "goal"})

	for _, test := range []struct {
		name       string
		actions    []core.Action
		goals      []*core.Goal
		conditions []core.Condition
	}{
		{name: "nil action", actions: []core.Action{nil}},
		{name: "typed nil action", actions: []core.Action{typedNilAction}},
		{name: "nil goal", goals: []*core.Goal{nil}},
		{name: "nil condition", conditions: []core.Condition{nil}},
		{name: "typed nil condition", goals: []*core.Goal{goal}, conditions: []core.Condition{typedNilCondition}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := planning.NewDomain(test.actions, test.goals, test.conditions); err == nil {
				t.Fatal("NewDomain accepted a nil member")
			}
		})
	}
}

func TestNewDomainRejectsNilMembersBeforeInspectingCapabilities(t *testing.T) {
	cause := errors.New("metadata must not run")
	_, err := planning.NewDomain(
		[]core.Action{panickingDomainAction{cause: cause}},
		[]*core.Goal{nil},
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "goal[0] is nil") || errors.Is(err, cause) {
		t.Fatalf("NewDomain error = %v, want nil goal before capability inspection", err)
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
	var typedNilPlanner plannerFunc
	if _, err := domain.Plans(t.Context(), typedNilPlanner, state, planning.Options{}); err == nil {
		t.Fatal("Plans accepted typed nil planner")
	}
	var typedNilState *planning.State
	if err := domain.ValidatePlanInputs(typedNilState, goal); err == nil {
		t.Fatal("ValidatePlanInputs accepted typed nil world state")
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
	goal := core.NewGoal(core.GoalConfig{Name: "goal", RequiredConditions: []string{"done"}})
	otherGoal := core.NewGoal(core.GoalConfig{Name: "other", RequiredConditions: []string{"done"}})
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

func TestDomainContainsPlannerResultMetadataPanic(t *testing.T) {
	cause := errors.New("candidate metadata sentinel")
	canonical := &planAction{metadata: core.ActionMetadata{Name: "work", Effects: core.ConditionSet{"done": core.True}}}
	goal := core.NewGoal(core.GoalConfig{Name: "goal", RequiredConditions: []string{"done"}})
	domain := mustDomain(t, []core.Action{canonical}, []*core.Goal{goal}, nil)

	_, err := domain.Plans(t.Context(), plannerFunc(func(goal *core.Goal) *planning.Plan {
		return planning.NewPlan([]core.Action{panickingDomainAction{cause: cause}}, goal)
	}), planning.NewState(nil), planning.Options{})
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), "planning: invalid plan") ||
		!strings.Contains(err.Error(), "Action.Metadata panicked") {
		t.Fatalf("Plans error = %v, want contained candidate metadata panic", err)
	}
}

func TestDomainCanonicalizesPlannerActions(t *testing.T) {
	canonical := &planAction{metadata: core.ActionMetadata{Name: "work", Effects: core.ConditionSet{"done": core.True}}}
	lookalike := &planAction{metadata: canonical.metadata}
	goal := core.NewGoal(core.GoalConfig{Name: "goal", RequiredConditions: []string{"done"}})
	domain := mustDomain(t, []core.Action{canonical}, []*core.Goal{goal}, nil)

	plans, err := domain.Plans(t.Context(), plannerFunc(func(goal *core.Goal) *planning.Plan {
		return planning.NewPlan([]core.Action{lookalike}, goal)
	}), planning.NewState(nil), planning.Options{})
	if err != nil {
		t.Fatalf("Plans: %v", err)
	}
	accepted := plans[0].Actions()[0]
	if accepted == lookalike || accepted.Metadata().Name != "work" {
		t.Fatalf("accepted action = %#v, want domain-owned canonical action", accepted)
	}
	if _, err := accepted.Execute(t.Context(), nil); err != nil || canonical.runs != 1 {
		t.Fatalf("canonical delegate runs/error = %d/%v", canonical.runs, err)
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
			goal := core.NewGoal(core.GoalConfig{Name: "goal", RequiredConditions: []string{"done"}, Value: test.goalValue})
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
