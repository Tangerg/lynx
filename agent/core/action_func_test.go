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
	groups := []core.ToolGroupRequirement{{
		Role:               "filesystem",
		AllowedPermissions: []core.ToolGroupPermission{core.ToolGroupHostAccess},
	}}
	action := core.NewAction[string, int](
		"defensive",
		func(context.Context, *core.ProcessContext, string) (int, error) { return 1, nil },
		core.ActionConfig{Inputs: inputs, Outputs: outputs, ToolGroups: groups},
	)

	inputs[0].Name = "mutated"
	outputs[0].Name = "mutated"
	groups[0].Role = "mutated"
	groups[0].AllowedPermissions[0] = core.ToolGroupInternetAccess

	metadata := action.Metadata()
	if metadata.Inputs[0].Name != "source" || metadata.Outputs[0].Name != "result" {
		t.Fatalf("constructor retained caller slices: %#v", metadata)
	}
	if metadata.ToolGroups[0].Role != "filesystem" || metadata.ToolGroups[0].AllowedPermissions[0] != core.ToolGroupHostAccess {
		t.Fatalf("constructor retained caller tool groups: %#v", metadata.ToolGroups)
	}

	metadata.Inputs[0].Name = "leaked"
	metadata.Effects[metadata.Outputs[0].String()] = core.False
	metadata.ToolGroups[0].AllowedPermissions[0] = core.ToolGroupInternetAccess
	again := action.Metadata()
	if again.Inputs[0].Name != "source" || again.Effects[again.Outputs[0].String()] != core.True {
		t.Fatalf("Metadata leaked stored maps/slices: %#v", again)
	}
	if again.ToolGroups[0].AllowedPermissions[0] != core.ToolGroupHostAccess {
		t.Fatalf("Metadata leaked nested permissions: %#v", again.ToolGroups)
	}
}

func TestNewActionDefaultsToOneAttempt(t *testing.T) {
	action := core.NewAction("once",
		func(context.Context, *core.ProcessContext, string) (int, error) { return 1, nil },
		core.ActionConfig{},
	)
	if got := action.Metadata().Retry; got != core.DefaultRetryPolicy() || got.MaxAttempts != 1 {
		t.Fatalf("default retry policy = %#v, want one attempt", got)
	}
}

type fakeBlackboard struct {
	value any
	ok    bool
}

func (f fakeBlackboard) Name() string { return "fake-blackboard" }
func (f fakeBlackboard) ID() string   { return "fake" }
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
func (f fakeBlackboard) Condition(string) (bool, bool) { return false, false }
func (f fakeBlackboard) Inspect(bool) string           { return "fake" }
func (f fakeBlackboard) Store(string, any)             {}
func (f fakeBlackboard) StoreTransient(string, any)    {}
func (f fakeBlackboard) Add(any)                       {}
func (f fakeBlackboard) AddTransient(any)              {}
func (f fakeBlackboard) Bind(any)                      {}
func (f fakeBlackboard) BindTransient(any)             {}
func (f fakeBlackboard) StoreAll(map[string]any)       {}
func (f fakeBlackboard) StoreProtected(string, any)    {}
func (f fakeBlackboard) Hide(any)                      {}
func (f fakeBlackboard) StoreCondition(string, bool)   {}
func (f fakeBlackboard) Clone() core.Blackboard        { return f }
func (f fakeBlackboard) ClearWorkingState()            {}
