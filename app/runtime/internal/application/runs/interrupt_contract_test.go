package runs

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

func TestResolveResumeResponsesValidatesExactTypedCoverage(t *testing.T) {
	approvalPending := interrupts.Pending{Interrupts: []transcript.Interrupt{{
		ItemID: "item_approval",
		Kind:   execution.ApprovalInterrupt,
		Approval: &transcript.Approval{
			Tool: transcript.ToolInvocation{Name: "shell"}, Rememberable: true,
		},
	}}, Suspensions: []interrupts.SuspensionBinding{{
		InterruptItemID: "item_approval", ProcessID: "process_approval", SuspensionID: "suspension_approval",
	}}}
	answers, err := resolveResumeResponses(approvalPending, []ResumeResponse{{
		ItemID: "item_approval",
		Kind:   ApprovalResponseKind,
		Approval: &ApprovalResponse{
			Approved: true, Arguments: `{"command":"echo edited"}`, RememberScope: approval.ScopeSession,
		},
	}})
	if err != nil {
		t.Fatalf("approval: %v", err)
	}
	if len(answers) != 1 {
		t.Fatalf("approval answers = %#v", answers)
	}
	resolution := answers[0].Resolution
	if !resolution.Approved || resolution.Arguments != `{"command":"echo edited"}` || resolution.RememberScope != approval.ScopeSession {
		t.Fatalf("approval resolution = %#v", resolution)
	}
	deniedAnswers, err := resolveResumeResponses(approvalPending, []ResumeResponse{{
		ItemID: "item_approval",
		Kind:   ApprovalResponseKind,
		Approval: &ApprovalResponse{
			Approved: false, Reason: "unsafe command",
		},
	}})
	if err != nil || len(deniedAnswers) != 1 {
		t.Fatalf("denial answers = %#v, %v", deniedAnswers, err)
	}
	denied := deniedAnswers[0].Resolution
	if denied.Approved || denied.Reason != "unsafe command" {
		t.Fatalf("denial resolution = %#v", denied)
	}

	questionPending := interrupts.Pending{Interrupts: []transcript.Interrupt{{
		ItemID: "item_question",
		Kind:   execution.QuestionInterrupt,
		Question: &transcript.Question{Fields: []transcript.QuestionField{{
			Name: "q0", Required: true, Kind: transcript.QuestionChoice,
			Options: []transcript.QuestionOption{{Label: "Go"}, {Label: "Stop"}},
		}}},
	}}, Suspensions: []interrupts.SuspensionBinding{{
		InterruptItemID: "item_question", ProcessID: "process_question", SuspensionID: "suspension_question",
	}}}
	answers, err = resolveResumeResponses(questionPending, []ResumeResponse{{
		ItemID: "item_question",
		Kind:   QuestionResponseKind,
		Question: &QuestionResponse{
			Answers: map[string][]string{"q0": {"Go"}},
		},
	}})
	if err != nil {
		t.Fatalf("question: %v", err)
	}
	resolution = answers[0].Resolution
	if !resolution.Approved || len(resolution.Answer["q0"]) != 1 || resolution.Answer["q0"][0] != "Go" {
		t.Fatalf("question resolution = %#v", resolution)
	}

	tests := []struct {
		name      string
		pending   interrupts.Pending
		responses []ResumeResponse
		want      error
	}{
		{name: "missing", pending: approvalPending, want: ErrInvalidInterruptResponse},
		{name: "unknown item", pending: approvalPending, responses: []ResumeResponse{{
			ItemID: "ghost", Kind: ApprovalResponseKind, Approval: &ApprovalResponse{Approved: true},
		}}, want: ErrInterruptNotOpen},
		{name: "wrong kind", pending: approvalPending, responses: []ResumeResponse{{
			ItemID: "item_approval", Kind: QuestionResponseKind, Question: &QuestionResponse{},
		}}, want: ErrInvalidInterruptResponse},
		{name: "duplicate", pending: approvalPending, responses: []ResumeResponse{
			{ItemID: "item_approval", Kind: ApprovalResponseKind, Approval: &ApprovalResponse{Approved: true}},
			{ItemID: "item_approval", Kind: ApprovalResponseKind, Approval: &ApprovalResponse{Approved: true}},
		}, want: ErrInvalidInterruptResponse},
		{name: "invalid choice", pending: questionPending, responses: []ResumeResponse{{
			ItemID: "item_question", Kind: QuestionResponseKind,
			Question: &QuestionResponse{Answers: map[string][]string{"q0": {"Rust"}}},
		}}, want: ErrInvalidInterruptResponse},
		{name: "one-off approval cannot be remembered", pending: interrupts.Pending{
			Interrupts: []transcript.Interrupt{{
				ItemID: "item_one_off", Kind: execution.ApprovalInterrupt,
				Approval: &transcript.Approval{Tool: transcript.ToolInvocation{Name: "shell"}},
			}},
			Suspensions: []interrupts.SuspensionBinding{{
				InterruptItemID: "item_one_off", ProcessID: "process_one_off", SuspensionID: "suspension_one_off",
			}},
		}, responses: []ResumeResponse{{
			ItemID: "item_one_off", Kind: ApprovalResponseKind,
			Approval: &ApprovalResponse{Approved: true, RememberScope: approval.ScopeSession},
		}}, want: ErrInvalidInterruptResponse},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := resolveResumeResponses(test.pending, test.responses)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestResolveResumeResponsesPreservesCompleteBarrierInCanonicalOrder(t *testing.T) {
	pending := interrupts.Pending{
		Interrupts: []transcript.Interrupt{
			{
				ItemID: "item_a",
				Kind:   execution.ApprovalInterrupt,
				Approval: &transcript.Approval{
					Tool: transcript.ToolInvocation{Name: "shell"},
				},
			},
			{
				ItemID: "item_b",
				Kind:   execution.ApprovalInterrupt,
				Approval: &transcript.Approval{
					Tool: transcript.ToolInvocation{Name: "write"},
				},
			},
		},
		Suspensions: []interrupts.SuspensionBinding{
			{InterruptItemID: "item_a", ProcessID: "process_a", SuspensionID: "suspension_a"},
			{InterruptItemID: "item_b", ProcessID: "process_b", SuspensionID: "suspension_b"},
		},
	}
	answers, err := resolveResumeResponses(pending, []ResumeResponse{
		{
			ItemID:   "item_b",
			Kind:     ApprovalResponseKind,
			Approval: &ApprovalResponse{Approved: false, Reason: "skip b"},
		},
		{
			ItemID:   "item_a",
			Kind:     ApprovalResponseKind,
			Approval: &ApprovalResponse{Approved: true},
		},
	})
	if err != nil {
		t.Fatalf("resolve complete barrier: %v", err)
	}
	if len(answers) != 2 {
		t.Fatalf("answers = %#v", answers)
	}
	if answers[0].InterruptItemID != "item_a" ||
		answers[0].ProcessID != "process_a" ||
		answers[0].SuspensionID != "suspension_a" ||
		!answers[0].Resolution.Approved {
		t.Fatalf("answer[0] = %#v", answers[0])
	}
	if answers[1].InterruptItemID != "item_b" ||
		answers[1].ProcessID != "process_b" ||
		answers[1].SuspensionID != "suspension_b" ||
		answers[1].Resolution.Approved ||
		answers[1].Resolution.Reason != "skip b" {
		t.Fatalf("answer[1] = %#v", answers[1])
	}
}
