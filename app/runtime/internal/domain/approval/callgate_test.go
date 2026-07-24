package approval

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

func TestToolCallInputPlan_HookBlockWins(t *testing.T) {
	plan := ToolCallInput{
		Tool:               "write",
		Arguments:          `{"file_path":"x"}`,
		Mode:               ModeYolo,
		ApprovalConfigured: true,
		Hook:               HookDecision{Block: true},
	}.Plan()
	if plan.Action != GateDeny || plan.Denial != (Denial{Cause: DenialHook}) {
		t.Fatalf("plan = %+v, want hook denial", plan)
	}
}

func TestToolCallInputPlan_RewritePassesAsOverride(t *testing.T) {
	plan := ToolCallInput{
		Tool:      "write",
		Arguments: `{"file_path":"unsafe"}`,
		Hook:      HookDecision{RewriteArguments: `{"file_path":"safe"}`},
	}.Plan()
	if plan.Action != GatePass || plan.Arguments != `{"file_path":"safe"}` || plan.ArgumentOverride != `{"file_path":"safe"}` {
		t.Fatalf("plan = %+v, want pass with rewritten override", plan)
	}
}

func TestToolCallInputPlan_HookAskEscalatesPass(t *testing.T) {
	plan := ToolCallInput{
		Tool:               "write",
		Arguments:          `{}`,
		Mode:               ModeBalanced,
		ApprovalConfigured: true,
		Hook:               HookDecision{Ask: true},
	}.Plan()
	if plan.Action != GatePrompt || plan.SafetyClass != tool.SafetyClassWrite || plan.Risk == "" || plan.PromptCause != PromptCauseWorkspaceWrite {
		t.Fatalf("plan = %+v, want prompt for hook ask", plan)
	}
}

func TestToolCallInputPlan_OutOfWorkspaceMutationIsBypassImmune(t *testing.T) {
	// Yolo would auto-pass a write, but a target outside the workspace must still
	// be confirmed — shown as high risk with a specific reason.
	plan := ToolCallInput{
		Tool:               "write",
		Arguments:          `{"file_path":"/etc/hosts"}`,
		Mode:               ModeYolo,
		ApprovalConfigured: true,
		FileMutation:       tool.FileMutationOutsideWorkspace,
	}.Plan()
	if plan.Action != GatePrompt {
		t.Fatalf("action = %v, want GatePrompt (bypass-immune)", plan.Action)
	}
	if plan.Risk != tool.RiskHigh || plan.PromptCause != PromptCauseOutsideWorkspace {
		t.Fatalf("plan = %+v, want high risk + out-of-workspace reason", plan)
	}
}

func TestToolCallInputPlan_InWorkspaceMutationStillAutoPassesInYolo(t *testing.T) {
	plan := ToolCallInput{
		Tool:               "write",
		Arguments:          `{"file_path":"src/a.go"}`,
		Mode:               ModeYolo,
		ApprovalConfigured: true,
		FileMutation:       tool.FileMutationWithinWorkspace,
	}.Plan()
	if plan.Action != GatePass {
		t.Fatalf("action = %v, want GatePass (in-workspace write under Yolo)", plan.Action)
	}
}

func TestToolCallInputPlan_NoMutationScopeDoesNotEscalate(t *testing.T) {
	plan := ToolCallInput{
		Tool:               "write",
		Arguments:          `{"file_path":"/etc/hosts"}`,
		Mode:               ModeYolo,
		ApprovalConfigured: true,
	}.Plan()
	if plan.Action != GatePass {
		t.Fatalf("action = %v, want GatePass (no workspace configured)", plan.Action)
	}
}

func TestToolCallInputPlan_ModePlanDenyBeatsHookAsk(t *testing.T) {
	plan := ToolCallInput{
		Tool:               "shell",
		Arguments:          `{}`,
		Mode:               ModePlan,
		ApprovalConfigured: true,
		Hook:               HookDecision{Ask: true},
	}.Plan()
	if plan.Action != GateDeny || plan.Denial.Cause != DenialPlanMode {
		t.Fatalf("plan = %+v, want plan-mode deny", plan)
	}
}

func TestToolCallPlanResolvePromptShortcuts_RememberedRuleBeforeAutoApprove(t *testing.T) {
	plan := ToolCallInput{
		Tool:               "srv_read",
		Arguments:          `{}`,
		Mode:               ModeSafe,
		ApprovalConfigured: true,
	}.Plan()
	got := plan.ResolvePromptShortcuts(StandingDecision{Matched: true, Decision: Deny}, true)
	if got.Action != GateDeny || got.Denial.Cause != DenialRememberedRule {
		t.Fatalf("remembered deny + auto approve = %+v, want deny", got)
	}

	got = plan.ResolvePromptShortcuts(StandingDecision{Matched: true, Decision: Allow}, false)
	if got.Action != GatePass {
		t.Fatalf("remembered allow = %+v, want pass", got)
	}

	got = plan.ResolvePromptShortcuts(StandingDecision{}, true)
	if got.Action != GatePass {
		t.Fatalf("auto approve = %+v, want pass", got)
	}
}
