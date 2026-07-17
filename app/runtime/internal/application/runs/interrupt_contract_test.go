package runs

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

func TestInterruptContractDiscriminatesAndRejectsGuesses(t *testing.T) {
	question := Interrupt{
		Kind: QuestionInterruptKind,
		Question: &QuestionPrompt{
			ToolName:  "ask_user",
			Arguments: `{"questions":[{"question":"Continue?"}]}`,
			Questions: []QuestionSpec{{
				Question: "Continue?",
				Options: []QuestionOptionSpec{
					{Label: "Yes"},
					{Label: "No"},
				},
			}},
		},
	}
	raw, err := json.Marshal(question)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeInterrupt(raw)
	if err != nil {
		t.Fatalf("DecodeInterrupt: %v", err)
	}
	if got.Kind != QuestionInterruptKind || got.Question == nil || got.Question.ToolName != "ask_user" {
		t.Fatalf("decoded = %#v", got)
	}

	approval := Interrupt{
		Kind: ApprovalInterruptKind,
		Approval: &ApprovalPrompt{
			ToolName: "webfetch", Arguments: `{"url":"https://example.com"}`,
			SafetyClass: tool.SafetyClassNetwork, Risk: tool.RiskHigh,
		},
	}
	raw, err = json.Marshal(approval)
	if err != nil {
		t.Fatal(err)
	}
	got, err = DecodeInterrupt(raw)
	if err != nil {
		t.Fatalf("DecodeInterrupt approval: %v", err)
	}
	if got.Approval == nil || got.Approval.SafetyClass != tool.SafetyClassNetwork || got.Approval.Risk != tool.RiskHigh {
		t.Fatalf("decoded approval = %#v", got.Approval)
	}

	for _, raw := range [][]byte{
		[]byte(`{"toolName":"shell","arguments":"{}"}`),
		[]byte(`{"kind":"future","approval":{"toolName":"shell","arguments":"{}","safetyClass":"exec"}}`),
		[]byte(`{"kind":"approval","approval":{"toolName":"shell","arguments":"{}","safetyClass":"exec"},"question":{"toolName":"ask_user","arguments":"{}","questions":[{"question":"x"}]}}`),
		[]byte(`{"kind":"question","question":{"toolName":"ask_user","arguments":"{}","questions":[]}}`),
		[]byte(`{"kind":"approval","approval":{"toolName":"shell","arguments":"not-json","safetyClass":"exec"}}`),
		[]byte(`{"kind":"approval","approval":{"toolName":"shell","arguments":"{}","safetyClass":"future"}}`),
		[]byte(`{"kind":"approval","approval":{"toolName":"shell","arguments":"{}","safetyClass":"exec","risk":"critical"}}`),
	} {
		if _, err := DecodeInterrupt(raw); err == nil {
			t.Errorf("DecodeInterrupt(%s) succeeded, want error", raw)
		}
	}
}

func TestResolveResumeResponsesValidatesExactTypedCoverage(t *testing.T) {
	approvalPending := interrupts.Pending{Interrupts: []transcript.Interrupt{{
		ItemID: "item_approval",
		Kind:   transcript.ApprovalInterrupt,
		Approval: &transcript.Approval{
			Tool: transcript.ToolInvocation{Name: "shell"},
		},
	}}}
	resolution, err := resolveResumeResponses(approvalPending, []ResumeResponse{{
		ItemID: "item_approval",
		Kind:   ApprovalResponseKind,
		Approval: &ApprovalResponse{
			Approved: true, Arguments: `{"command":"echo edited"}`, RememberScope: "session",
		},
	}})
	if err != nil {
		t.Fatalf("approval: %v", err)
	}
	if !resolution.Approved || resolution.Arguments != `{"command":"echo edited"}` || resolution.RememberScope != "session" {
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
