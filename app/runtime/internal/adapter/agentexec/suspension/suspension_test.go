package suspension

import (
	"encoding/json"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

func TestDecodePromptDiscriminatesAndRejectsGuesses(t *testing.T) {
	question := runs.Interrupt{
		Kind: runs.QuestionInterruptKind,
		Question: &runs.QuestionPrompt{
			ToolName:  "ask_user",
			Arguments: `{"questions":[{"question":"Continue?"}]}`,
			Questions: []runs.QuestionSpec{{
				Question: "Continue?",
				Options:  []runs.QuestionOptionSpec{{Label: "Yes"}, {Label: "No"}},
			}},
		},
	}
	raw, err := EncodePrompt(question)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodePrompt(raw)
	if err != nil {
		t.Fatalf("DecodePrompt: %v", err)
	}
	if got.Kind != runs.QuestionInterruptKind || got.Question == nil || got.Question.ToolName != "ask_user" {
		t.Fatalf("decoded = %#v", got)
	}

	approval := runs.Interrupt{
		Kind: runs.ApprovalInterruptKind,
		Approval: &runs.ApprovalPrompt{
			ToolName: "webfetch", Arguments: `{"url":"https://example.com"}`,
			SafetyClass: tool.SafetyClassNetwork, Risk: tool.RiskHigh,
		},
	}
	raw, err = EncodePrompt(approval)
	if err != nil {
		t.Fatal(err)
	}
	got, err = DecodePrompt(raw)
	if err != nil {
		t.Fatalf("DecodePrompt approval: %v", err)
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
		if _, err := DecodePrompt(raw); err == nil {
			t.Errorf("DecodePrompt(%s) succeeded, want error", raw)
		}
	}
}

func TestResolutionCodecUsesAgentWireVocabulary(t *testing.T) {
	raw, err := EncodeResolution(interrupts.Resolution{
		Approved: true, Arguments: `{"command":"go test"}`, RememberScope: approval.ScopeSession,
	})
	if err != nil {
		t.Fatalf("EncodeResolution: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("decode encoded response: %v", err)
	}
	if wire["approved"] != true || wire["remember_scope"] != "session" {
		t.Fatalf("response wire = %#v", wire)
	}
	if _, found := wire["Approved"]; found {
		t.Fatalf("response leaked Go field name: %#v", wire)
	}
	decoded, err := DecodeResolution(raw)
	if err != nil || decoded.RememberScope != approval.ScopeSession {
		t.Fatalf("DecodeResolution = %#v, %v", decoded, err)
	}
}
