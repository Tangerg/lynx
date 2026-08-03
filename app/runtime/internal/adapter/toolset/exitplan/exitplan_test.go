package exitplan

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
)

type modeStore struct {
	states map[string]approval.SessionMode
}

func (s *modeStore) GetMode(_ context.Context, sessionID string) (approval.SessionMode, bool, error) {
	state, found := s.states[sessionID]
	return state, found, nil
}

func (s *modeStore) PutMode(_ context.Context, sessionID string, state approval.SessionMode) error {
	s.states[sessionID] = state
	return nil
}

type planReader struct{ steps []plan.Step }

func (r planReader) List(context.Context, string) ([]plan.Step, error) { return r.steps, nil }

func planContext(t *testing.T, sessionID string) context.Context {
	t.Helper()
	return executionctx.WithScope(t.Context(), execution.TurnScope{SessionID: sessionID})
}

func planPolicy(t *testing.T, mode approval.Mode) *approval.RuntimePolicy {
	t.Helper()
	policy, err := approval.New(mode, nil, &modeStore{states: make(map[string]approval.SessionMode)})
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func TestNewRequiresModeAndPlanPorts(t *testing.T) {
	policy := planPolicy(t, approval.ModeBalanced)
	for _, build := range []func() (any, error){
		func() (any, error) { return New(nil, planReader{}, nil) },
		func() (any, error) { return New(policy, nil, nil) },
	} {
		got, err := build()
		if err != nil {
			t.Fatal(err)
		}
		if got != nil {
			t.Fatal("missing port should omit exit_plan_mode")
		}
	}
}

func TestDefinitionHasNoSecondPlanInput(t *testing.T) {
	tool, err := New(planPolicy(t, approval.ModeBalanced), planReader{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := tool.Definition().Name; got != "exit_plan_mode" {
		t.Fatalf("name = %q", got)
	}
	schema := string(tool.Definition().InputSchema)
	if strings.Contains(schema, `"plan"`) || strings.Contains(schema, `"options"`) {
		t.Fatalf("schema exposes a second Plan value: %s", schema)
	}
	if _, err := tool.Call(t.Context(), `{"plan":"shadow copy"}`); err == nil {
		t.Fatal("obsolete Plan input was accepted")
	}
}

func TestExitRequiresSessionAndPlanMode(t *testing.T) {
	policy := planPolicy(t, approval.ModeBalanced)
	tool, err := New(policy, planReader{steps: []plan.Step{{Description: "inspect", Status: plan.StatusPending}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tool.Call(t.Context(), `{}`); err == nil || !strings.Contains(err.Error(), "no active session") {
		t.Fatalf("missing-session error = %v", err)
	}
	if _, err := tool.Call(planContext(t, "session-1"), `{}`); err == nil || !strings.Contains(err.Error(), "not in Plan mode") {
		t.Fatalf("non-Plan error = %v", err)
	}
}

func TestExitRejectsEmptyCanonicalPlan(t *testing.T) {
	policy := planPolicy(t, approval.ModeBalanced)
	if _, err := policy.EnterPlanMode(t.Context(), "session-1"); err != nil {
		t.Fatal(err)
	}
	tool, err := New(policy, planReader{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tool.Call(planContext(t, "session-1"), `{}`); err == nil || !strings.Contains(err.Error(), "current Plan is empty") {
		t.Fatalf("empty Plan error = %v", err)
	}
}

func TestRejectionKeepsPlanMode(t *testing.T) {
	const sessionID = "session-reject"
	policy := planPolicy(t, approval.ModeBalanced)
	if _, err := policy.EnterPlanMode(t.Context(), sessionID); err != nil {
		t.Fatal(err)
	}
	interrupt := func(_ context.Context, _ string, pending runs.Interrupt) (interrupts.Resolution, error) {
		if pending.Question == nil || !strings.Contains(pending.Question.Fields[0].Prompt, "inspect") {
			t.Fatalf("prompt does not present the canonical Plan: %+v", pending.Question)
		}
		return interrupts.Resolution{Answers: [][]string{{rejectLabel}}}, nil
	}
	tool, err := New(policy, planReader{steps: []plan.Step{{Description: "inspect", Status: plan.StatusInProgress}}}, interrupt)
	if err != nil {
		t.Fatal(err)
	}
	result, err := tool.Call(planContext(t, sessionID), `{}`)
	if err != nil || !strings.Contains(result, "remains in Plan mode") {
		t.Fatalf("Call = %q, %v", result, err)
	}
	if mode, _ := policy.Mode(t.Context(), sessionID); mode != approval.ModePlan {
		t.Fatalf("mode = %v, want Plan", mode)
	}
}

func TestApprovalRestoresModeCapturedOnEntry(t *testing.T) {
	const sessionID = "session-approve"
	policy := planPolicy(t, approval.ModeBalanced)
	if _, err := policy.EnterPlanMode(t.Context(), sessionID); err != nil {
		t.Fatal(err)
	}
	if err := policy.SetDefaultMode(t.Context(), approval.ModeYolo); err != nil {
		t.Fatal(err)
	}
	interrupt := func(_ context.Context, _ string, pending runs.Interrupt) (interrupts.Resolution, error) {
		if got := pending.Question.Arguments; got != `{}` {
			t.Fatalf("interrupt arguments = %q", got)
		}
		return interrupts.Resolution{Answers: [][]string{{approveLabel}}}, nil
	}
	tool, err := New(policy, planReader{steps: []plan.Step{{Description: "implement", Status: plan.StatusPending}}}, interrupt)
	if err != nil {
		t.Fatal(err)
	}
	result, err := tool.Call(planContext(t, sessionID), `{}`)
	if err != nil || !strings.Contains(result, "balanced was restored") {
		t.Fatalf("Call = %q, %v", result, err)
	}
	if mode, _ := policy.Mode(t.Context(), sessionID); mode != approval.ModeBalanced {
		t.Fatalf("mode = %v, want captured Balanced", mode)
	}
}
