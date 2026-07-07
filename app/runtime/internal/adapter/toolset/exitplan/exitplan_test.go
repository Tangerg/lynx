package exitplan

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
)

// TestNew_NilApproval: no approval policy → no tool (omitted).
func TestNew_NilApproval(t *testing.T) {
	if New(nil) != nil {
		t.Error("New(nil) should yield a nil tool")
	}
}

// TestExitPlan_Validation: malformed args and an empty plan are model-facing
// errors raised before the call parks.
func TestExitPlan_Validation(t *testing.T) {
	tool := New(approval.New(approval.ModePlan, nil))
	if _, err := tool.Call(context.Background(), `not json`); err == nil {
		t.Error("invalid JSON must error")
	}
	if _, err := tool.Call(context.Background(), `{"plan":"  "}`); err == nil {
		t.Error("blank plan must error")
	}
}

// TestExitPlan_NotInPlanMode: calling exit_plan_mode outside the plan stance is
// a no-op message (not an error, no park) — it only applies in plan mode.
func TestExitPlan_NotInPlanMode(t *testing.T) {
	tool := New(approval.New(approval.ModeBalanced, nil)) // not plan
	out, err := tool.Call(context.Background(), `{"plan":"do the thing"}`)
	if err != nil {
		t.Fatalf("err=%v, want a graceful not-in-plan message", err)
	}
	if !strings.Contains(out, "Not in plan mode") {
		t.Errorf("out=%q, want a not-in-plan message", out)
	}
}
