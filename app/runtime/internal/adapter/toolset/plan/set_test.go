package plan

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	plandomain "github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
)

type stubStore struct{ steps []plandomain.Step }

func (s *stubStore) Replace(_ context.Context, _ string, steps []plandomain.Step) error {
	s.steps = steps
	return nil
}

func TestNewNilStoreOmitsTool(t *testing.T) {
	tool, err := newSet(nil)
	if err != nil {
		t.Fatal(err)
	}
	if tool != nil {
		t.Fatal("New(nil) returned a tool")
	}
}

func TestSetPlanDefinition(t *testing.T) {
	tool, err := newSet(&stubStore{})
	if err != nil {
		t.Fatal(err)
	}
	if got := tool.Definition().Name; got != "set_plan" {
		t.Fatalf("tool name = %q, want set_plan", got)
	}
	schema := string(tool.Definition().InputSchema)
	for _, want := range []string{`"steps"`, `"description"`, `"in_progress"`} {
		if !strings.Contains(schema, want) {
			t.Fatalf("input schema = %s, missing %s", schema, want)
		}
	}
}

func TestSetPlanRequiresSession(t *testing.T) {
	tool, err := newSet(&stubStore{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tool.Call(context.Background(), `{"steps":[{"description":"inspect","status":"pending"}]}`); err == nil || !strings.Contains(err.Error(), "no active session") {
		t.Fatalf("Call error = %v, want no active session", err)
	}
}

func TestSetPlanRejectsBadArguments(t *testing.T) {
	tool, err := newSet(&stubStore{})
	if err != nil {
		t.Fatal(err)
	}
	for _, arguments := range []string{
		`{not json`,
		`{}`,
		`{"steps":[{"description":"inspect","status":"done"}]}`,
	} {
		if _, err := tool.Call(context.Background(), arguments); err == nil {
			t.Errorf("Call(%s) succeeded", arguments)
		}
	}
}

func TestSetPlanReplacesAndClearsTheSessionPlan(t *testing.T) {
	store := &stubStore{}
	tool, err := newSet(store)
	if err != nil {
		t.Fatal(err)
	}
	ctx := executionctx.WithScope(t.Context(), runs.ExecutionScope{SessionID: "session-1"})

	result, err := tool.Call(ctx, `{"steps":[{"description":"inspect","status":"in_progress"}]}`)
	if err != nil {
		t.Fatalf("set Plan: %v", err)
	}
	if result != "Plan updated:\n[~] inspect\n" || len(store.steps) != 1 || store.steps[0].Description != "inspect" {
		t.Fatalf("result = %q, stored = %+v", result, store.steps)
	}

	result, err = tool.Call(ctx, `{"steps":[]}`)
	if err != nil {
		t.Fatalf("clear Plan: %v", err)
	}
	if result != "Plan cleared." || len(store.steps) != 0 {
		t.Fatalf("clear result = %q, stored = %+v", result, store.steps)
	}
}

func TestSetArgsMapsSteps(t *testing.T) {
	steps := (setArgs{Steps: []stepArg{{Description: "debug failing test", Status: "in_progress"}}}).steps()
	want := []plandomain.Step{{Description: "debug failing test", Status: plandomain.StatusInProgress}}
	if len(steps) != 1 || steps[0] != want[0] {
		t.Fatalf("steps = %+v, want %+v", steps, want)
	}
}
