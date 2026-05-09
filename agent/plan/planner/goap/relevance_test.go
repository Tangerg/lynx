package goap

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

// stubAction is a minimal Action used to assemble unit tests of the
// regression pruner without depending on the typed-action machinery.
type stubAction struct {
	meta core.ActionMetadata
}

func (s stubAction) Metadata() core.ActionMetadata { return s.meta }
func (s stubAction) Execute(context.Context, *core.ProcessContext) core.ActionStatus {
	return core.ActionSucceeded
}

func newStubAction(name string, pre, eff core.EffectSpec) stubAction {
	return stubAction{
		meta: core.ActionMetadata{
			Name:          name,
			Preconditions: pre,
			Effects:       eff,
		},
	}
}

func actionNames(actions []core.Action) []string {
	names := make([]string, len(actions))
	for i, a := range actions {
		names[i] = a.Metadata().Name
	}
	return names
}

func contains(slice []string, want string) bool {
	for _, s := range slice {
		if s == want {
			return true
		}
	}
	return false
}

func TestRelevantActions_KeepsDirectGoalProducers(t *testing.T) {
	produceX := newStubAction("produceX",
		core.EffectSpec{},
		core.EffectSpec{"x": core.True},
	)
	unrelated := newStubAction("unrelated",
		core.EffectSpec{},
		core.EffectSpec{"y": core.True},
	)

	goal := &core.Goal{Pre: []string{"x"}}

	got := actionNames(relevantActions(
		[]core.Action{produceX, unrelated}, goal,
	))
	if !contains(got, "produceX") {
		t.Fatalf("relevant should include produceX: %v", got)
	}
	if contains(got, "unrelated") {
		t.Fatalf("relevant should exclude unrelated (its effect y doesn't help reach x): %v", got)
	}
}

func TestRelevantActions_TransitivelyIncludesEnablers(t *testing.T) {
	// Goal needs c=True. produceC requires b=True. produceB requires
	// a=True. produceA has no preconditions. The full chain should be
	// in the relevant set; producesD (effect d=True) should be excluded.
	produceA := newStubAction("produceA", core.EffectSpec{}, core.EffectSpec{"a": core.True})
	produceB := newStubAction("produceB", core.EffectSpec{"a": core.True}, core.EffectSpec{"b": core.True})
	produceC := newStubAction("produceC", core.EffectSpec{"b": core.True}, core.EffectSpec{"c": core.True})
	produceD := newStubAction("produceD", core.EffectSpec{}, core.EffectSpec{"d": core.True})

	goal := &core.Goal{Pre: []string{"c"}}

	got := actionNames(relevantActions(
		[]core.Action{produceA, produceB, produceC, produceD}, goal,
	))
	for _, want := range []string{"produceA", "produceB", "produceC"} {
		if !contains(got, want) {
			t.Fatalf("relevant chain should include %s: %v", want, got)
		}
	}
	if contains(got, "produceD") {
		t.Fatalf("produceD's d=True is not reachable backward from c: %v", got)
	}
}

func TestRelevantActions_EmptyWhenNoProducer(t *testing.T) {
	produceY := newStubAction("produceY",
		core.EffectSpec{},
		core.EffectSpec{"y": core.True},
	)

	// Goal needs x=True; no action produces x → relevant set must be empty.
	goal := &core.Goal{Pre: []string{"x"}}
	got := relevantActions([]core.Action{produceY}, goal)
	if len(got) != 0 {
		t.Fatalf("relevant should be empty when no producer for goal precondition: %v", actionNames(got))
	}
}

func TestRelevantActions_DistinguishesValuePerKey(t *testing.T) {
	// Goal wants x=True. setXTrue produces x=True. setXFalse produces
	// x=False. Only setXTrue should be relevant — setXFalse's effect
	// doesn't match any (key, value) the regression needs.
	setXTrue := newStubAction("setXTrue", core.EffectSpec{}, core.EffectSpec{"x": core.True})
	setXFalse := newStubAction("setXFalse", core.EffectSpec{}, core.EffectSpec{"x": core.False})

	goal := &core.Goal{Pre: []string{"x"}}

	got := actionNames(relevantActions([]core.Action{setXTrue, setXFalse}, goal))
	if !contains(got, "setXTrue") {
		t.Fatalf("setXTrue should be relevant: %v", got)
	}
	if contains(got, "setXFalse") {
		t.Fatalf("setXFalse produces x=False (different value); should be excluded: %v", got)
	}
}

func TestRelevantActions_PreservesInputOrder(t *testing.T) {
	a := newStubAction("a", core.EffectSpec{}, core.EffectSpec{"x": core.True})
	b := newStubAction("b", core.EffectSpec{}, core.EffectSpec{"x": core.True})
	c := newStubAction("c", core.EffectSpec{}, core.EffectSpec{"x": core.True})

	goal := &core.Goal{Pre: []string{"x"}}

	got := actionNames(relevantActions([]core.Action{a, b, c}, goal))
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("relevantActions should preserve input order: %v", got)
	}
}

func TestRelevantActions_NilSafe(t *testing.T) {
	produceX := newStubAction("produceX",
		core.EffectSpec{},
		core.EffectSpec{"x": core.True},
	)

	goal := &core.Goal{Pre: []string{"x"}}

	got := actionNames(relevantActions(
		[]core.Action{nil, produceX, nil}, goal,
	))
	if !contains(got, "produceX") || len(got) != 1 {
		t.Fatalf("nil entries should be filtered: %v", got)
	}
}
