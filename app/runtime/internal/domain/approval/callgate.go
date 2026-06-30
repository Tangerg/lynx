package approval

import "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"

// HookDecision is the approval-relevant part of a PreToolUse hook decision.
type HookDecision struct {
	Block            bool
	Reason           string
	Ask              bool
	RewriteArguments string
}

// StandingDecision is a remembered approval rule matched for this call.
type StandingDecision struct {
	Decision Decision
	Matched  bool
}

// ToolCallInput is the pure policy input for one tool call.
type ToolCallInput struct {
	Tool               string
	Arguments          string
	Mode               Mode
	ApprovalConfigured bool
	Hook               HookDecision
}

// ToolCallPlan is the approval policy's verdict before any HITL interrupt is
// executed. Action is pass/deny/prompt; Arguments is the effective call payload
// after hook rewrite; ArgumentOverride is non-empty only when the engine should
// replace the original tool arguments.
type ToolCallPlan struct {
	Action           GateAction
	Arguments        string
	ArgumentOverride string
	DenyReason       string
	SafetyClass      tool.SafetyClass
	Risk             string
	PromptReason     string
}

// PlanToolCall applies hook and approval-mode policy to one tool call. It does
// not read remembered rules and it does not trigger HITL; callers only do those
// side effects when the returned plan asks for [GatePrompt].
func PlanToolCall(in ToolCallInput) ToolCallPlan {
	arguments := in.Arguments
	override := ""
	if in.Hook.RewriteArguments != "" {
		arguments = in.Hook.RewriteArguments
		override = in.Hook.RewriteArguments
	}
	cls := tool.SafetyClassFor(in.Tool)
	plan := ToolCallPlan{
		Action:           GatePass,
		Arguments:        arguments,
		ArgumentOverride: override,
		SafetyClass:      cls,
	}
	if in.Hook.Block {
		plan.Action = GateDeny
		plan.DenyReason = firstNonEmpty(in.Hook.Reason, "denied by a PreToolUse hook")
		return plan
	}
	if !in.ApprovalConfigured {
		return plan
	}

	action := GateFor(cls, in.Mode)
	if in.Hook.Ask && action == GatePass {
		action = GatePrompt
	}
	plan.Action = action
	switch action {
	case GateDeny:
		plan.DenyReason = planModeDenyReason(in.Tool)
	case GatePrompt:
		plan.Risk, plan.PromptReason = RiskFor(cls)
	}
	return plan
}

// ResolvePromptShortcuts applies non-HITL prompt short-circuits: remembered
// rules first, then an explicit auto-approve grant. It is a no-op unless plan
// is [GatePrompt].
func ResolvePromptShortcuts(plan ToolCallPlan, standing StandingDecision, autoApproved bool) ToolCallPlan {
	if plan.Action != GatePrompt {
		return plan
	}
	if standing.Matched {
		if standing.Decision == Deny {
			plan.Action = GateDeny
			plan.DenyReason = "tool call denied by a remembered rule"
			return plan
		}
		plan.Action = GatePass
		return plan
	}
	if autoApproved {
		plan.Action = GatePass
	}
	return plan
}

// ApprovedArguments returns the tool-argument override after a human approval:
// edited arguments win, otherwise a hook rewrite is preserved.
func (p ToolCallPlan) ApprovedArguments(edited string) string {
	return firstNonEmpty(edited, p.ArgumentOverride)
}

// DecisionOf maps an approve/deny boolean to the approval domain's verdict.
func DecisionOf(approved bool) Decision {
	if approved {
		return Allow
	}
	return Deny
}

func planModeDenyReason(toolName string) string {
	return "plan mode is active (read-only): " + toolName + " is not permitted. Investigate with read-only tools, then call exit_plan_mode to present your plan for approval."
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
