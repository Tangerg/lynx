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
	Arguments    string
	Mode         Mode
	Hook         HookDecision
	SafetyClass  tool.SafetyClass
	FileMutation tool.FileMutationScope
	ShellCommand string
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
// text; generated wording belongs to the caller that presents the denial.
type Denial struct {
	Cause  DenialCause
	Detail string
}

// DenialCause is the policy source of a refusal.
type DenialCause string

const (
	DenialNone           DenialCause = ""
	DenialHook           DenialCause = "hook"
	DenialPlanMode       DenialCause = "planMode"
	DenialRememberedRule DenialCause = "rememberedRule"
)

// Valid reports whether d belongs to the denial taxonomy.
func (d DenialCause) Valid() bool {
	return d == DenialNone || d == DenialHook || d == DenialPlanMode || d == DenialRememberedRule
}

// PromptCause is the policy fact an approval surface explains to a user.
type PromptCause string

const (
	PromptCauseNone                PromptCause = ""
	PromptCauseNonMutating         PromptCause = "nonMutating"
	PromptCauseWorkspaceWrite      PromptCause = "workspaceWrite"
	PromptCauseWorkspaceCommand    PromptCause = "workspaceCommand"
	PromptCauseNetworkAccess       PromptCause = "networkAccess"
	PromptCauseUnknownSafety       PromptCause = "unknownSafety"
	PromptCauseOutsideWorkspace    PromptCause = "outsideWorkspace"
	PromptCauseUnknownMutation     PromptCause = "unknownMutation"
	PromptCauseCatastrophicCommand PromptCause = "catastrophicCommand"
)

// Valid reports whether p belongs to the approval prompt taxonomy.
func (p PromptCause) Valid() bool {
	return p == PromptCauseNone || p == PromptCauseNonMutating || p == PromptCauseWorkspaceWrite ||
		p == PromptCauseWorkspaceCommand || p == PromptCauseNetworkAccess || p == PromptCauseUnknownSafety ||
		p == PromptCauseOutsideWorkspace || p == PromptCauseUnknownMutation || p == PromptCauseCatastrophicCommand
}

// Plan applies hook and approval-mode policy to one tool call. It does not
// read remembered rules and it does not trigger HITL; callers only do those
// side effects when the returned plan asks for [GatePrompt].
func (t ToolCallInput) Plan() ToolCallPlan {
	arguments := t.Arguments
	override := ""
	if t.Hook.RewriteArguments != "" {
		arguments = t.Hook.RewriteArguments
		override = t.Hook.RewriteArguments
	}
	plan := ToolCallPlan{
		Action:           GatePass,
		Arguments:        arguments,
		ArgumentOverride: override,
		SafetyClass:      t.SafetyClass,
	}
	if t.Hook.Block {
		plan.Action = GateDeny
		plan.Denial = Denial{Cause: DenialHook, Detail: t.Hook.Reason}
		return plan
	}
	action := GateFor(t.SafetyClass, t.Mode)
	// Bypass-immune escalation: a call dangerous enough (a mutation escaping the
	// workspace, or a high-confidence catastrophic shell command) is confirmed
	// even under a mode that would auto-pass it (Yolo, or Balanced for
	// file mutations). This override is not defeated by "approve everything" — the
	// same seam a PreToolUse hook's Ask uses to force a prompt, but
	// tool/argument-driven and built in. A remembered approval still lets a repeat
	// call through.
	immunity := tool.BypassImmunityFor(t.FileMutation, t.ShellCommand)
	if action == GatePass && (t.Hook.Ask || immunity != tool.BypassAllowed) {
		action = GatePrompt
	}
	plan.Action = action
	switch action {
	case GateDeny:
		plan.Denial = Denial{Cause: DenialPlanMode}
	case GatePrompt:
		plan.Risk = t.SafetyClass.Risk()
		plan.PromptCause = promptCauseForSafetyClass(t.SafetyClass)
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
func (t ToolCallPlan) ResolvePromptShortcuts(standing StandingDecision, autoApproved bool) ToolCallPlan {
	if t.Action != GatePrompt {
		return t
	}
	if standing.Matched {
		if standing.Decision == Deny {
			t.Action = GateDeny
			t.Denial = Denial{Cause: DenialRememberedRule}
			return t
		}
		t.Action = GatePass
		return t
	}
	if autoApproved {
		t.Action = GatePass
	}
	return t
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
		return PromptCauseNonMutating
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
