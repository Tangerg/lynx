package terminal

import (
	"slices"

	"github.com/Tangerg/oolong/components/headless"

	"github.com/Tangerg/scope/app/cli/internal/agent"
)

// approvalAction is the terminal's complete decision vocabulary for one tool
// approval. Keeping it closed prevents form choices and runtime answers from
// drifting as new persistence scopes are exposed.
type approvalAction string

const (
	approvalAllowOnce    approvalAction = "allow-once"
	approvalAllowSession approvalAction = "allow-session"
	approvalAllowProject approvalAction = "allow-project"
	approvalAllowGlobal  approvalAction = "allow-global"
	approvalDenyOnce     approvalAction = "deny-once"
	approvalDenySession  approvalAction = "deny-session"
	approvalDenyProject  approvalAction = "deny-project"
	approvalDenyGlobal   approvalAction = "deny-global"
	approvalEditArgs     approvalAction = "edit-arguments"
)

func approvalOptions(rememberable bool) []headless.Option[approvalAction] {
	options := []headless.Option[approvalAction]{
		{Label: "Allow once", Value: approvalAllowOnce},
		{Label: "Deny once", Value: approvalDenyOnce},
		{Label: "Edit arguments before deciding", Value: approvalEditArgs},
	}
	if !rememberable {
		return options
	}
	options = slices.Insert(options, 1,
		headless.Option[approvalAction]{Label: "Allow for this session", Value: approvalAllowSession},
		headless.Option[approvalAction]{Label: "Allow for this project", Value: approvalAllowProject},
		headless.Option[approvalAction]{Label: "Always allow this rule", Value: approvalAllowGlobal},
	)
	return slices.Insert(options, len(options)-1,
		headless.Option[approvalAction]{Label: "Deny for this session", Value: approvalDenySession},
		headless.Option[approvalAction]{Label: "Deny for this project", Value: approvalDenyProject},
		headless.Option[approvalAction]{Label: "Always deny this rule", Value: approvalDenyGlobal},
	)
}

func (a approvalAction) Normalize(rememberable bool) approvalAction {
	for _, option := range approvalOptions(rememberable) {
		if option.Value == a {
			return a
		}
	}
	return approvalAllowOnce
}

func defaultApprovalAction(scope agent.RememberScope) approvalAction {
	switch scope {
	case agent.RememberSession:
		return approvalAllowSession
	case agent.RememberProject:
		return approvalAllowProject
	case agent.RememberGlobal:
		return approvalAllowGlobal
	default:
		return approvalAllowOnce
	}
}

func (a approvalAction) Answer() (agent.ApprovalAnswer, bool) {
	switch a {
	case approvalAllowSession:
		return agent.ApprovalAnswer{Decision: agent.ApprovalApprove, Remember: agent.RememberSession}, true
	case approvalAllowProject:
		return agent.ApprovalAnswer{Decision: agent.ApprovalApprove, Remember: agent.RememberProject}, true
	case approvalAllowGlobal:
		return agent.ApprovalAnswer{Decision: agent.ApprovalApprove, Remember: agent.RememberGlobal}, true
	case approvalAllowOnce:
		return agent.ApprovalAnswer{Decision: agent.ApprovalApprove, Remember: agent.RememberNone}, true
	case approvalDenySession:
		return agent.ApprovalAnswer{Decision: agent.ApprovalDeny, Remember: agent.RememberSession}, true
	case approvalDenyProject:
		return agent.ApprovalAnswer{Decision: agent.ApprovalDeny, Remember: agent.RememberProject}, true
	case approvalDenyGlobal:
		return agent.ApprovalAnswer{Decision: agent.ApprovalDeny, Remember: agent.RememberGlobal}, true
	case approvalDenyOnce:
		return agent.ApprovalAnswer{Decision: agent.ApprovalDeny, Remember: agent.RememberNone}, true
	default:
		return agent.ApprovalAnswer{}, false
	}
}

func approvalActionFromAnswer(answer agent.ApprovalAnswer) approvalAction {
	if answer.Decision == agent.ApprovalDeny {
		switch answer.Remember {
		case agent.RememberSession:
			return approvalDenySession
		case agent.RememberProject:
			return approvalDenyProject
		case agent.RememberGlobal:
			return approvalDenyGlobal
		default:
			return approvalDenyOnce
		}
	}
	switch answer.Remember {
	case agent.RememberSession:
		return approvalAllowSession
	case agent.RememberProject:
		return approvalAllowProject
	case agent.RememberGlobal:
		return approvalAllowGlobal
	default:
		return approvalAllowOnce
	}
}
