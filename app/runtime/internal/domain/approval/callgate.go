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
	Denial           Denial
	SafetyClass      tool.SafetyClass
	Risk             tool.RiskLevel
	PromptCause      PromptCause
}

// Denial identifies why the gate refused a call. Detail preserves hook-owned
// text; generated wording belongs to the adapter that presents the denial.
type Denial struct {
	Cause  DenialCause
	Detail string
}

// DenialCause is the policy source of a refusal.
type DenialCause uint8

const (
	DenialNone DenialCause = iota
	DenialHook
	DenialPlanMode
	DenialRememberedRule
)

// PromptCause is the policy fact an approval surface explains to a user.
type PromptCause uint8

const (
	PromptCauseNone PromptCause = iota
	PromptCauseRead
	PromptCauseWorkspaceWrite
	PromptCauseWorkspaceCommand
	PromptCauseNetworkAccess
	PromptCauseUnknownSafety
	PromptCauseOutsideWorkspace
	PromptCauseUnknownMutation
	PromptCauseCatastrophicCommand
)

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
		plan.Denial = Denial{Cause: DenialHook, Detail: in.Hook.Reason}
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
	immunity := tool.BypassImmunityFor(in.FileMutation, in.ShellCommand)
	if action == GatePass && (in.Hook.Ask || immunity != tool.BypassAllowed) {
		action = GatePrompt
	}
	plan.Action = action
	switch action {
	case GateDeny:
		plan.Denial = Denial{Cause: DenialPlanMode}
	case GatePrompt:
		plan.Risk = cls.Risk()
		plan.PromptCause = promptCauseForSafetyClass(cls)
		if immunity != tool.BypassAllowed {
			plan.Risk = tool.RiskHigh
			plan.PromptCause = promptCauseForBypassImmunity(immunity)
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
			p.Denial = Denial{Cause: DenialRememberedRule}
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

// DecisionOf maps an approve/deny boolean to the approval domain's verdict.
func DecisionOf(approved bool) Decision {
	if approved {
		return Allow
	}
	return Deny
}

func promptCauseForSafetyClass(class tool.SafetyClass) PromptCause {
	switch class {
	case tool.SafetyClassSafe:
		return PromptCauseRead
	case tool.SafetyClassWrite:
		return PromptCauseWorkspaceWrite
	case tool.SafetyClassExec:
		return PromptCauseWorkspaceCommand
	case tool.SafetyClassNetwork:
		return PromptCauseNetworkAccess
	default:
		return PromptCauseUnknownSafety
	}
}

func promptCauseForBypassImmunity(immunity tool.BypassImmunity) PromptCause {
	switch immunity {
	case tool.BypassImmuneOutsideWorkspace:
		return PromptCauseOutsideWorkspace
	case tool.BypassImmuneUnknownMutation:
		return PromptCauseUnknownMutation
	case tool.BypassImmuneCatastrophicCommand:
		return PromptCauseCatastrophicCommand
	default:
		return PromptCauseNone
	}
}
