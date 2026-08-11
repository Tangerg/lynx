package terminal

import (
	"testing"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func TestApprovalChoiceMapsEveryDecisionAndRememberScope(t *testing.T) {
	tests := []struct {
		name     string
		choice   string
		decision agent.ApprovalDecision
		remember agent.RememberScope
	}{
		{name: "allow once", choice: "allow-once", decision: agent.ApprovalApprove, remember: agent.RememberNone},
		{name: "allow session", choice: "allow-session", decision: agent.ApprovalApprove, remember: agent.RememberSession},
		{name: "allow project", choice: "allow-project", decision: agent.ApprovalApprove, remember: agent.RememberProject},
		{name: "allow global", choice: "allow-global", decision: agent.ApprovalApprove, remember: agent.RememberGlobal},
		{name: "deny", choice: "deny", decision: agent.ApprovalDeny, remember: agent.RememberNone},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			answer := approvalAnswer(test.choice)
			if answer.Decision != test.decision || answer.Remember != test.remember {
				t.Fatalf("approvalAnswer(%q) = %+v", test.choice, answer)
			}
		})
	}
}

func TestApprovalDefaultSelectsEveryConfiguredRememberScope(t *testing.T) {
	tests := []struct {
		scope agent.RememberScope
		want  string
	}{
		{scope: agent.RememberNone, want: "allow-once"},
		{scope: agent.RememberSession, want: "allow-session"},
		{scope: agent.RememberProject, want: "allow-project"},
		{scope: agent.RememberGlobal, want: "allow-global"},
	}
	for _, test := range tests {
		if got := approvalDefault(test.scope); got != test.want {
			t.Errorf("approvalDefault(%q) = %q, want %q", test.scope, got, test.want)
		}
	}
}
