package core_test

import (
	"context"
	"slices"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

func TestCompositeConditionsHandleNilChildren(t *testing.T) {
	if got := core.And(nil, core.NewCondition(core.ConditionConfig{Name: "ready"})).Evaluate(t.Context(), nil); got != core.Unknown {
		t.Fatalf("And(nil, ready) = %s, want unknown", got)
	}
	if got := core.Or(nil, core.NewCondition(core.ConditionConfig{Name: "ready"})).Evaluate(t.Context(), nil); got != core.Unknown {
		t.Fatalf("Or(nil, ready) = %s, want unknown", got)
	}
	if got := core.Not(nil).Evaluate(t.Context(), nil); got != core.Unknown {
		t.Fatalf("Not(nil) = %s, want unknown", got)
	}
	var typedNil *core.FuncCondition
	if got := core.And(typedNil, core.NewCondition(core.ConditionConfig{Name: "ready"})).Evaluate(t.Context(), nil); got != core.Unknown {
		t.Fatalf("And(typed nil, ready) = %s, want unknown", got)
	}
	if got := core.Not(typedNil).Name(); got != "(NOT <nil>)" {
		t.Fatalf("Not(typed nil) name = %q", got)
	}
}

func TestCompositeConditionsEvaluateCheaperOperandFirst(t *testing.T) {
	tests := []struct {
		name      string
		composite func(core.Condition, core.Condition) core.Condition
		cheap     core.Truth
		expensive core.Truth
		want      core.Truth
	}{
		{name: "and short-circuits false", composite: core.And, cheap: core.False, expensive: core.True, want: core.False},
		{name: "or short-circuits true", composite: core.Or, cheap: core.True, expensive: core.False, want: core.True},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var evaluations []string
			expensive := core.NewCondition(core.ConditionConfig{Name: "expensive", EvaluationCost: 100, Evaluate: func(context.Context, *core.ConditionEnv) core.Truth {
				evaluations = append(evaluations, "expensive")
				return test.expensive
			}})
			cheap := core.NewCondition(core.ConditionConfig{Name: "cheap", EvaluationCost: 1, Evaluate: func(context.Context, *core.ConditionEnv) core.Truth {
				evaluations = append(evaluations, "cheap")
				return test.cheap
			}})

			if got := test.composite(expensive, cheap).Evaluate(t.Context(), nil); got != test.want {
				t.Fatalf("Evaluate = %s, want %s", got, test.want)
			}
			if !slices.Equal(evaluations, []string{"cheap"}) {
				t.Fatalf("evaluations = %v, want only cheap", evaluations)
			}
		})
	}
}

func TestNewConditionCarriesEvaluationCost(t *testing.T) {
	condition := core.NewCondition(core.ConditionConfig{Name: "remote", EvaluationCost: 7.5})
	if condition.Name() != "remote" || condition.EvaluationCost() != 7.5 {
		t.Fatalf("condition = %q/%v, want remote/7.5", condition.Name(), condition.EvaluationCost())
	}
}

func TestCompositeConditionNamesHandleNilAndUnnamedChildren(t *testing.T) {
	if got := core.And(nil, core.NewCondition(core.ConditionConfig{})).Name(); got != "(<nil> AND <unnamed>)" {
		t.Fatalf("And name = %q", got)
	}
	// And and Or share everything about their name but the operator, so this is
	// the assertion that keeps them from rendering as each other.
	if got := core.Or(nil, core.NewCondition(core.ConditionConfig{})).Name(); got != "(<nil> OR <unnamed>)" {
		t.Fatalf("Or name = %q", got)
	}
	if got := core.Not(nil).Name(); got != "(NOT <nil>)" {
		t.Fatalf("Not name = %q", got)
	}
}
