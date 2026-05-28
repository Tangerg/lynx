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
		ProcessState: core.ProcessState{
			Blackboard: fakeBlackboard{
				value: "hello",
				ok:    true,
			},
		},
	})

	if got := action.Execute(t.Context(), processContext); got != core.ActionFailed {
		t.Fatalf("Execute status = %s, want failed", got)
	}
	if err := processContext.LastError(); err == nil || !strings.Contains(err.Error(), "action function is nil") {
		t.Fatalf("LastError = %v, want nil function message", err)
	}
}

func TestTypedActionReportsMissingInput(t *testing.T) {
	action := core.NewAction[string, int]("needs-input",
		func(context.Context, *core.ProcessContext, string) (int, error) { return 0, nil },
		core.ActionConfig{},
	)
	processContext := core.NewProcessContext(core.ProcessContextConfig{
		ProcessState: core.ProcessState{
			Blackboard: fakeBlackboard{},
		},
	})

	if got := action.Execute(t.Context(), processContext); got != core.ActionFailed {
		t.Fatalf("Execute status = %s, want failed", got)
	}
	if err := processContext.LastError(); err == nil || !strings.Contains(err.Error(), "blackboard is missing required input") {
		t.Fatalf("LastError = %v, want missing input message", err)
	}
}

func TestTypedActionReportsInputTypeMismatch(t *testing.T) {
	action := core.NewAction[string, int]("needs-string",
		func(context.Context, *core.ProcessContext, string) (int, error) { return 0, nil },
		core.ActionConfig{},
	)
	processContext := core.NewProcessContext(core.ProcessContextConfig{
		ProcessState: core.ProcessState{
			Blackboard: fakeBlackboard{
				value: 42,
				ok:    true,
			},
		},
	})

	if got := action.Execute(t.Context(), processContext); got != core.ActionFailed {
		t.Fatalf("Execute status = %s, want failed", got)
	}
	if err := processContext.LastError(); err == nil || !strings.Contains(err.Error(), "has type int") {
		t.Fatalf("LastError = %v, want type mismatch message", err)
	}
}

func TestExecuteSafelyRejectsNilAction(t *testing.T) {
	processContext := core.NewProcessContext(core.ProcessContextConfig{})

	if got := processContext.ExecuteSafely(t.Context(), nil); got != core.ActionFailed {
		t.Fatalf("ExecuteSafely status = %s, want failed", got)
	}
	if err := processContext.LastError(); err == nil || !strings.Contains(err.Error(), "action is nil") {
		t.Fatalf("LastError = %v, want nil action message", err)
	}
}

type fakeBlackboard struct {
	value any
	ok    bool
}

func (f fakeBlackboard) Name() string { return "fake-blackboard" }
func (f fakeBlackboard) ID() string   { return "fake" }
func (f fakeBlackboard) Get(string) (any, bool) {
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
func (f fakeBlackboard) Set(string, any)               {}
func (f fakeBlackboard) AddObject(any)                 {}
func (f fakeBlackboard) Bind(any)                      {}
func (f fakeBlackboard) BindAll(map[string]any)        {}
func (f fakeBlackboard) BindProtected(string, any)     {}
func (f fakeBlackboard) Hide(any)                      {}
func (f fakeBlackboard) SetCondition(string, bool)     {}
func (f fakeBlackboard) Spawn() core.Blackboard        { return f }
func (f fakeBlackboard) Clear()                        {}
