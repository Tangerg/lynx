package autonomy_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/plan"
	"github.com/Tangerg/lynx/agent/runtime/autonomy"
	"github.com/Tangerg/lynx/core/model/chat"
)

func newPlan(goalName string, value, cost float64) *plan.Plan {
	return &plan.Plan{
		Goal: &core.Goal{
			Name:  goalName,
			Value: core.Static(value),
			Pre:   []string{"x"},
		},
		// Empty Actions — Plan.Cost sums over Action.Metadata().Cost,
		// so we shadow with a fake action that reports cost.
		Actions: []core.Action{
			&fakePlanAction{cost: cost},
		},
	}
}

type fakePlanAction struct {
	cost float64
}

func (a *fakePlanAction) Metadata() core.ActionMetadata {
	return core.ActionMetadata{
		Name: "fake",
		Cost: core.Static(a.cost),
	}
}
func (a *fakePlanAction) Execute(context.Context, *core.ProcessContext) core.ActionStatus {
	return core.ActionFailed
}

func TestLLMPlanRanker_ReordersByLLMConfidence(t *testing.T) {
	plans := []*plan.Plan{
		newPlan("low", 1, 0.1),
		newPlan("medium", 5, 1),
		newPlan("high", 10, 2),
	}

	// LLM scores middle plan (plan_1) highest — that should win
	// regardless of NetValue ordering.
	reply := `{"choices":[
  {"id":"plan_0","confidence":0.1,"rationale":""},
  {"id":"plan_1","confidence":0.9,"rationale":"perfect fit"},
  {"id":"plan_2","confidence":0.4,"rationale":"too costly"}
]}`
	model := newStubModel(reply)
	client, _ := chat.NewClient(model)

	ranker, _ := autonomy.NewLLMPlanRanker(client, autonomy.LLMPlanRankerConfig{})
	out, err := ranker.Rank(t.Context(), plans, plan.EmptyWorldState())
	if err != nil {
		t.Fatalf("Rank: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("expected 3 plans, got %d", len(out))
	}
	if out[0].Goal.Name != "medium" {
		t.Fatalf("expected medium first, got %s", out[0].Goal.Name)
	}
	if out[1].Goal.Name != "high" {
		t.Fatalf("expected high second, got %s", out[1].Goal.Name)
	}
	if out[2].Goal.Name != "low" {
		t.Fatalf("expected low last, got %s", out[2].Goal.Name)
	}
}

func TestLLMPlanRanker_PreservesOrderForSinglePlan(t *testing.T) {
	plans := []*plan.Plan{newPlan("only", 1, 1)}
	model := newStubModel("ignored — never called")
	client, _ := chat.NewClient(model)
	ranker, _ := autonomy.NewLLMPlanRanker(client, autonomy.LLMPlanRankerConfig{})

	out, err := ranker.Rank(t.Context(), plans, plan.EmptyWorldState())
	if err != nil {
		t.Fatalf("Rank: %v", err)
	}
	if len(out) != 1 || out[0].Goal.Name != "only" {
		t.Fatalf("expected single-plan passthrough, got %+v", out)
	}
}

func TestLLMPlanRanker_PromptContainsPlanSummaries(t *testing.T) {
	plans := []*plan.Plan{
		newPlan("first", 2, 1),
		newPlan("second", 5, 2),
	}
	model := newStubModel(`{"choices":[
  {"id":"plan_0","confidence":0.5,"rationale":""},
  {"id":"plan_1","confidence":0.6,"rationale":""}
]}`)
	client, _ := chat.NewClient(model)

	ranker, _ := autonomy.NewLLMPlanRanker(client, autonomy.LLMPlanRankerConfig{})
	if _, err := ranker.Rank(t.Context(), plans, plan.EmptyWorldState()); err != nil {
		t.Fatalf("Rank: %v", err)
	}

	if !strings.Contains(model.gotPrompt, "plan_0") || !strings.Contains(model.gotPrompt, "plan_1") {
		t.Fatalf("prompt should list plan ids; got %s", model.gotPrompt)
	}
	if !strings.Contains(model.gotPrompt, `goal="first"`) ||
		!strings.Contains(model.gotPrompt, `goal="second"`) {
		t.Fatalf("prompt should list goal names; got %s", model.gotPrompt)
	}
}

func TestLLMPlanRanker_RejectsNilClient(t *testing.T) {
	if _, err := autonomy.NewLLMPlanRanker(nil, autonomy.LLMPlanRankerConfig{}); err == nil {
		t.Fatal("expected error")
	}
}
