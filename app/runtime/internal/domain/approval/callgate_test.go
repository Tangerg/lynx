package approval

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

func TestPlanToolCall_HookBlockWins(t *testing.T) {
	plan := PlanToolCall(ToolCallInput{
		Tool:               "write",
		Arguments:          `{"file_path":"x"}`,
		Mode:               ModeYolo,
		ApprovalConfigured: true,
		Hook:               HookDecision{Block: true},
	})
	if plan.Action != GateDeny || plan.DenyReason != "denied by a PreToolUse hook" {
		t.Fatalf("plan = %+v, want hook denial", plan)
	}
}

func TestPlanToolCall_RewritePassesAsOverride(t *testing.T) {
	plan := PlanToolCall(ToolCallInput{
		Tool:      "write",
		Arguments: `{"file_path":"unsafe"}`,
		Hook:      HookDecision{RewriteArguments: `{"file_path":"safe"}`},
	})
	if plan.Action != GatePass || plan.Arguments != `{"file_path":"safe"}` || plan.ArgumentOverride != `{"file_path":"safe"}` {
		t.Fatalf("plan = %+v, want pass with rewritten override", plan)
	}
}

func TestPlanToolCall_HookAskEscalatesPass(t *testing.T) {
	plan := PlanToolCall(ToolCallInput{
		Tool:               "write",
		Arguments:          `{}`,
		Mode:               ModeBalanced,
		ApprovalConfigured: true,
		Hook:               HookDecision{Ask: true},
	})
	if plan.Action != GatePrompt || plan.SafetyClass != tool.SafetyClassWrite || plan.Risk == "" || plan.PromptReason == "" {
		t.Fatalf("plan = %+v, want prompt for hook ask", plan)
	}
}

func TestPlanToolCall_ModePlanDenyBeatsHookAsk(t *testing.T) {
	plan := PlanToolCall(ToolCallInput{
		Tool:               "shell",
		Arguments:          `{}`,
		Mode:               ModePlan,
		ApprovalConfigured: true,
		Hook:               HookDecision{Ask: true},
	})
	if plan.Action != GateDeny || !strings.Contains(plan.DenyReason, "plan mode") {
		t.Fatalf("plan = %+v, want plan-mode deny", plan)
	}
}

func TestResolvePromptShortcuts_RememberedRuleBeforeAutoApprove(t *testing.T) {
	plan := PlanToolCall(ToolCallInput{
		Tool:               "srv_read",
		Arguments:          `{}`,
		Mode:               ModeSafe,
		ApprovalConfigured: true,
	})
	got := ResolvePromptShortcuts(plan, StandingDecision{Matched: true, Decision: Deny}, true)
	if got.Action != GateDeny || got.DenyReason != "tool call denied by a remembered rule" {
		t.Fatalf("remembered deny + auto approve = %+v, want deny", got)
	}

	got = ResolvePromptShortcuts(plan, StandingDecision{Matched: true, Decision: Allow}, false)
	if got.Action != GatePass {
		t.Fatalf("remembered allow = %+v, want pass", got)
	}

	got = ResolvePromptShortcuts(plan, StandingDecision{}, true)
	if got.Action != GatePass {
		t.Fatalf("auto approve = %+v, want pass", got)
	}
}

func TestToolCallPlan_ApprovedArguments(t *testing.T) {
	plan := ToolCallPlan{ArgumentOverride: `{"path":"hook"}`}
	if got := plan.ApprovedArguments(`{"path":"human"}`); got != `{"path":"human"}` {
		t.Fatalf("human edit = %q, want human args", got)
	}
	if got := plan.ApprovedArguments(""); got != `{"path":"hook"}` {
		t.Fatalf("no human edit = %q, want hook rewrite", got)
	}
}
