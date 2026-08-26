package builtin

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
)

type modePolicy struct {
	sessions []string
	changed  bool
}

func (m *modePolicy) EnterPlanMode(_ context.Context, sessionID string) (bool, error) {
	m.sessions = append(m.sessions, sessionID)
	return m.changed, nil
}

func TestNewNilPolicyOmitsTool(t *testing.T) {
	tool, err := newEnter(nil)
	if err != nil {
		t.Fatal(err)
	}
	if tool != nil {
		t.Fatal("New(nil) returned a tool")
	}
}

func TestDefinitionUsesEmptyInput(t *testing.T) {
	tool, err := newEnter(&modePolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if got := tool.Definition().Name; got != "enter_plan_mode" {
		t.Fatalf("name = %q", got)
	}
	if schema := string(tool.Definition().InputSchema); strings.Contains(schema, `"mode"`) || strings.Contains(schema, `"plan"`) {
		t.Fatalf("schema contains invented inputs: %s", schema)
	}
	if _, err := tool.Call(t.Context(), `{"mode":"plan"}`); err == nil {
		t.Fatal("obsolete mode input was accepted")
	}
}

func TestEnterRequiresSession(t *testing.T) {
	tool, err := newEnter(&modePolicy{changed: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tool.Call(t.Context(), `{}`); err == nil || !strings.Contains(err.Error(), "no active session") {
		t.Fatalf("error = %v", err)
	}
}

func TestEnterUsesCurrentSession(t *testing.T) {
	policy := &modePolicy{changed: true}
	tool, err := newEnter(policy)
	if err != nil {
		t.Fatal(err)
	}
	ctx := executionctx.WithScope(t.Context(), runs.ExecutionScope{SessionID: "session-1"})
	result, err := tool.Call(ctx, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(policy.sessions) != 1 || policy.sessions[0] != "session-1" {
		t.Fatalf("sessions = %v", policy.sessions)
	}
	if !strings.Contains(result, "Plan mode entered") || !strings.Contains(result, "set_plan") {
		t.Fatalf("result = %q", result)
	}
}

func TestAlreadyEnteredIsIdempotent(t *testing.T) {
	tool, err := newEnter(&modePolicy{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := executionctx.WithScope(t.Context(), runs.ExecutionScope{SessionID: "session-1"})
	result, err := tool.Call(ctx, `{}`)
	if err != nil || !strings.Contains(result, "already in Plan mode") {
		t.Fatalf("Call = %q, %v", result, err)
	}
}
