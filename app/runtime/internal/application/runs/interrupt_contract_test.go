package runs

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

func TestResolveResumeResponsesValidatesExactTypedCoverage(t *testing.T) {
	approvalPending := interrupts.Pending{Interrupts: []transcript.Interrupt{{
		ItemID: "item_approval",
		Kind:   transcript.ApprovalInterrupt,
		Approval: &transcript.Approval{
			Tool: transcript.ToolInvocation{Name: "shell"}, Rememberable: true,
		},
	}}}
	resolution, err := resolveResumeResponses(approvalPending, []ResumeResponse{{
		ItemID: "item_approval",
		Kind:   ApprovalResponseKind,
		Approval: &ApprovalResponse{
			Approved: true, Arguments: `{"command":"echo edited"}`, RememberScope: approval.ScopeSession,
		},
	}})
	if err != nil {
		t.Fatalf("approval: %v", err)
	}
	if !resolution.Approved || resolution.Arguments != `{"command":"echo edited"}` || resolution.RememberScope != approval.ScopeSession {
		t.Fatalf("approval resolution = %#v", resolution)
	}
	denied, err := resolveResumeResponses(approvalPending, []ResumeResponse{{
		ItemID: "item_approval",
		Kind:   ApprovalResponseKind,
		Approval: &ApprovalResponse{
			Approved: false, Reason: "unsafe command",
		},
	}})
	if err != nil || denied.Approved || denied.Reason != "unsafe command" {
		t.Fatalf("denial resolution = %#v, %v", denied, err)
	}

	questionPending := interrupts.Pending{Interrupts: []transcript.Interrupt{{
		ItemID: "item_question",
		Kind:   transcript.QuestionInterrupt,
		Question: &transcript.Question{Fields: []transcript.QuestionField{{
			Name: "q0", Required: true, Kind: transcript.QuestionChoice,
			Options: []transcript.QuestionOption{{Label: "Go"}, {Label: "Stop"}},
		}}},
	}}}
	resolution, err = resolveResumeResponses(questionPending, []ResumeResponse{{
		ItemID: "item_question",
		Kind:   QuestionResponseKind,
		Question: &QuestionResponse{
			Answers: map[string][]string{"q0": {"Go"}},
		},
	}})
	if err != nil {
		t.Fatalf("question: %v", err)
	}
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
		{name: "one-off approval cannot be remembered", pending: interrupts.Pending{Interrupts: []transcript.Interrupt{{
			ItemID: "item_one_off", Kind: transcript.ApprovalInterrupt,
			Approval: &transcript.Approval{Tool: transcript.ToolInvocation{Name: "shell"}},
		}}}, responses: []ResumeResponse{{
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
