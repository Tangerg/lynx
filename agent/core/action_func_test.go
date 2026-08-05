package core_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

func TestTypedActionRejectsNilFunction(t *testing.T) {
	action := core.NewAction[string, int]("nil-fn", nil, core.ActionConfig{})
	processContext := core.NewProcessContext(core.ProcessContextConfig{
		Blackboard: fakeBlackboard{
			value: "hello",
			ok:    true,
		},
	})

	status, err := action.Execute(t.Context(), processContext)
	if status != core.ActionFailed {
		t.Fatalf("Execute status = %s, want failed", status)
	}
	if err == nil || !strings.Contains(err.Error(), "action function is nil") {
		t.Fatalf("Execute error = %v, want nil function message", err)
	}
}

func TestTypedActionReportsMissingInput(t *testing.T) {
	action := core.NewAction[string, int]("needs-input",
		func(context.Context, *core.ProcessContext, string) (int, error) { return 0, nil },
		core.ActionConfig{},
	)
	processContext := core.NewProcessContext(core.ProcessContextConfig{
		Blackboard: fakeBlackboard{},
	})

	status, err := action.Execute(t.Context(), processContext)
	if status != core.ActionFailed {
		t.Fatalf("Execute status = %s, want failed", status)
	}
	if err == nil || !strings.Contains(err.Error(), "blackboard is missing required input") {
		t.Fatalf("Execute error = %v, want missing input message", err)
	}
}

func TestTypedActionReportsInputTypeMismatch(t *testing.T) {
	action := core.NewAction[string, int]("needs-string",
		func(context.Context, *core.ProcessContext, string) (int, error) { return 0, nil },
		core.ActionConfig{},
	)
	processContext := core.NewProcessContext(core.ProcessContextConfig{
		Blackboard: fakeBlackboard{
			value: 42,
			ok:    true,
		},
	})

	status, err := action.Execute(t.Context(), processContext)
	if status != core.ActionFailed {
		t.Fatalf("Execute status = %s, want failed", status)
	}
	if err == nil || !strings.Contains(err.Error(), "has type int") {
		t.Fatalf("Execute error = %v, want type mismatch message", err)
	}
}

func TestTypedActionMetadataIsDefensive(t *testing.T) {
	inputs := []core.Binding{{Name: "source", Type: "string"}}
	outputs := []core.Binding{{Name: "result", Type: "int"}}
	groups := []string{"filesystem"}
	action := core.NewAction[string, int](
		"defensive",
		func(context.Context, *core.ProcessContext, string) (int, error) { return 1, nil },
		core.ActionConfig{Inputs: inputs, Outputs: outputs, ToolGroups: groups},
	)

	inputs[0].Name = "mutated"
	outputs[0].Name = "mutated"
	groups[0] = "mutated"

	metadata := action.Metadata()
	if metadata.Inputs[0].Name != "source" || metadata.Outputs[0].Name != "result" {
		t.Fatalf("constructor retained caller slices: %#v", metadata)
	}
	if metadata.ToolGroups[0] != "filesystem" {
		t.Fatalf("constructor retained caller tool groups: %#v", metadata.ToolGroups)
	}

	metadata.Inputs[0].Name = "leaked"
	metadata.Effects[metadata.Outputs[0].String()] = core.False
	metadata.ToolGroups[0] = "leaked"
	again := action.Metadata()
	if again.Inputs[0].Name != "source" || again.Effects[again.Outputs[0].String()] != core.True ||
		again.ToolGroups[0] != "filesystem" {
		t.Fatalf("Metadata leaked stored maps/slices: %#v", again)
	}
}

func TestActionMetadataCloneOwnsCompositeValues(t *testing.T) {
	original := core.ActionMetadata{
		Inputs:        []core.Binding{{Name: "input", Type: "string"}},
		Outputs:       []core.Binding{{Name: "output", Type: "string"}},
		Preconditions: core.ConditionSet{"ready": core.True},
		Effects:       core.ConditionSet{"done": core.True},
		ToolGroups:    []string{"workspace"},
		Cost:          core.FixedScore(1),
	}
	cloned := original.Clone()
	cloned.Inputs[0].Name = "mutated"
	cloned.Outputs[0].Name = "mutated"
	cloned.Preconditions["ready"] = core.False
	cloned.Effects["done"] = core.False
	cloned.ToolGroups[0] = "mutated"

	if original.Inputs[0].Name != "input" || original.Outputs[0].Name != "output" ||
		original.Preconditions["ready"] != core.True || original.Effects["done"] != core.True ||
		original.ToolGroups[0] != "workspace" {
		t.Fatalf("Clone shared composite values with source: %#v", original)
	}
}

type fakeBlackboard struct {
	value any
	ok    bool
	clone func() (core.Blackboard, error)
}

func (f fakeBlackboard) Name() string { return "fake-blackboard" }
func (f fakeBlackboard) Load(string) (any, bool) {
	return f.value, f.ok
}
func (f fakeBlackboard) Lookup(string, string) (any, bool) {
	return f.value, f.ok
}
func (f fakeBlackboard) HasValue(string, string) bool { return f.ok }
func (f fakeBlackboard) Objects() []any {
	if !f.ok {
		return nil
	}
	return []any{f.value}
}
func (f fakeBlackboard) Condition(string) (bool, bool)     { return false, false }
func (f fakeBlackboard) Store(string, any) error           { return nil }
func (f fakeBlackboard) Add(any) error                     { return nil }
func (f fakeBlackboard) Bind(any) error                    { return nil }
func (f fakeBlackboard) StoreAll(core.Bindings) error      { return nil }
func (f fakeBlackboard) Hide(any) error                    { return nil }
func (f fakeBlackboard) StoreCondition(string, bool) error { return nil }
func (f fakeBlackboard) Clone() (core.Blackboard, error) {
	if f.clone != nil {
		return f.clone()
	}
	return f, nil
}
func (f fakeBlackboard) ClearWorkingState() error { return nil }
