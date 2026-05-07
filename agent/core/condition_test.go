package core_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

func TestCompositeConditionsHandleNilChildren(t *testing.T) {
	if got := core.And(nil, core.NewCondition("ready", nil)).Evaluate(context.Background(), nil); got != core.Unknown {
		t.Fatalf("And(nil, ready) = %s, want unknown", got)
	}
	if got := core.Or(nil, core.NewCondition("ready", nil)).Evaluate(context.Background(), nil); got != core.Unknown {
		t.Fatalf("Or(nil, ready) = %s, want unknown", got)
	}
	if got := core.Not(nil).Evaluate(context.Background(), nil); got != core.Unknown {
		t.Fatalf("Not(nil) = %s, want unknown", got)
	}
}

func TestCompositeConditionNamesHandleNilAndUnnamedChildren(t *testing.T) {
	if got := core.And(nil, core.NewCondition("", nil)).Name(); got != "(<nil> AND <unnamed>)" {
		t.Fatalf("And name = %q", got)
	}
	if got := core.Not(nil).Name(); got != "(NOT <nil>)" {
		t.Fatalf("Not name = %q", got)
	}
}
