package approval

import (
	"cmp"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

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
	FileMutation       tool.FileMutationScope
	ShellCommand       string
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
	Risk             tool.RiskLevel
	PromptReason     string
}

// Plan applies hook and approval-mode policy to one tool call. It does not
// read remembered rules and it does not trigger HITL; callers only do those
// side effects when the returned plan asks for [GatePrompt].
func (in ToolCallInput) Plan() ToolCallPlan {
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
		plan.DenyReason = cmp.Or(in.Hook.Reason, "denied by a PreToolUse hook")
		return plan
	}
	if !in.ApprovalConfigured {
		return plan
	}

	action := GateFor(cls, in.Mode)
	// Bypass-immune escalation: a call dangerous enough (a mutation escaping the
	// workspace, or a high-confidence catastrophic shell command) is confirmed
	// even under a mode that would auto-pass it (Yolo, or Balanced for
	// write/download). This override is not defeated by "approve everything" — the
	// same seam a PreToolUse hook's Ask uses to force a prompt, but
	// tool/argument-driven and built in. A remembered approval still lets a repeat
	// call through.
	immuneReason, unbypassable := tool.BypassImmuneReason(in.FileMutation, in.ShellCommand)
	if action == GatePass && (in.Hook.Ask || unbypassable) {
		action = GatePrompt
	}
	plan.Action = action
	switch action {
	case GateDeny:
		plan.DenyReason = planModeDenyReason(in.Tool)
	case GatePrompt:
		plan.Risk, plan.PromptReason = RiskFor(cls)
		if unbypassable {
			plan.Risk, plan.PromptReason = tool.RiskHigh, immuneReason
		}
	}
	return plan
}

// ResolvePromptShortcuts applies non-HITL prompt short-circuits: remembered
// rules first, then an explicit auto-approve grant. It is a no-op unless the
// plan is [GatePrompt].
func (p ToolCallPlan) ResolvePromptShortcuts(standing StandingDecision, autoApproved bool) ToolCallPlan {
	if p.Action != GatePrompt {
		return p
	}
	if standing.Matched {
		if standing.Decision == Deny {
			p.Action = GateDeny
			p.DenyReason = "tool call denied by a remembered rule"
			return p
		}
		p.Action = GatePass
		return p
	}
	if autoApproved {
		p.Action = GatePass
	}
	return p
}

// ApprovedArguments returns the tool-argument override after a human approval:
// edited arguments win, otherwise a hook rewrite is preserved.
func (p ToolCallPlan) ApprovedArguments(edited string) string {
	return cmp.Or(edited, p.ArgumentOverride)
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
