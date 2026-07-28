package core_test

import (
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

func TestCompositeConditionsHandleNilChildren(t *testing.T) {
	if got := core.And(nil, core.NewCondition("ready", nil)).Evaluate(t.Context(), nil); got != core.Unknown {
		t.Fatalf("And(nil, ready) = %s, want unknown", got)
	}
	if got := core.Or(nil, core.NewCondition("ready", nil)).Evaluate(t.Context(), nil); got != core.Unknown {
		t.Fatalf("Or(nil, ready) = %s, want unknown", got)
	}
	if got := core.Not(nil).Evaluate(t.Context(), nil); got != core.Unknown {
		t.Fatalf("Not(nil) = %s, want unknown", got)
	}
}

func TestCompositeConditionNamesHandleNilAndUnnamedChildren(t *testing.T) {
	if got := core.And(nil, core.NewCondition("", nil)).Name(); got != "(<nil> AND <unnamed>)" {
		t.Fatalf("And name = %q", got)
	}
	// And and Or share everything about their name but the operator, so this is
	// the assertion that keeps them from rendering as each other.
	if got := core.Or(nil, core.NewCondition("", nil)).Name(); got != "(<nil> OR <unnamed>)" {
		t.Fatalf("Or name = %q", got)
	}
	if got := core.Not(nil).Name(); got != "(NOT <nil>)" {
		t.Fatalf("Not name = %q", got)
	}
}
