package turn

import (
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
)

func approvalDenialMessage(denial approval.Denial, toolName string) string {
	switch denial.Cause {
	case approval.DenialHook:
		if denial.Detail != "" {
			return denial.Detail
		}
		return "denied by a PreToolUse hook"
	case approval.DenialPlanMode:
		return fmt.Sprintf("plan mode is active (read-only): %s is not permitted. Investigate with read-only tools, then call exit_plan_mode to present your plan for approval.", toolName)
	case approval.DenialRememberedRule:
		return "tool call denied by a remembered rule"
	default:
		return "tool call denied by approval policy"
	}
}

func approvalPromptReason(cause approval.PromptCause) string {
	switch cause {
	case approval.PromptCauseRead:
		return "Reads data without changing the workspace."
	case approval.PromptCauseWorkspaceWrite:
		return "Modifies files in the workspace."
	case approval.PromptCauseWorkspaceCommand:
		return "Runs commands in the workspace."
	case approval.PromptCauseNetworkAccess:
		return "Accesses network resources."
	case approval.PromptCauseOutsideWorkspace:
		return "Targets a path outside the workspace directory."
	case approval.PromptCauseUnknownMutation:
		return "Has filesystem mutation targets that could not be verified."
	case approval.PromptCauseCatastrophicCommand:
		return "Runs a high-confidence catastrophic shell command."
	default:
		return "Has an unknown safety classification."
	}
}
