package suspension

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

func TestDecodePromptDiscriminatesAndRejectsGuesses(t *testing.T) {
	question := runs.Interrupt{
		Kind: interrupt.Question,
		Question: &runs.QuestionPrompt{
			ToolName:  "ask_user",
			Arguments: `{"questions":[{"question":"Continue?"}]}`,
			Fields: []runs.QuestionFieldSpec{{
				Prompt: "Continue?", AllowCustom: true,
				Options: []runs.QuestionOptionSpec{{Label: "Yes"}, {Label: "No"}},
			}},
		},
	}
	raw, err := json.Marshal(promptWireFrom(question))
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(raw)
	if !strings.Contains(encoded, `"fields"`) || !strings.Contains(encoded, `"prompt"`) ||
		strings.Contains(encoded, `"multiSelect"`) {
		t.Fatalf("question checkpoint uses stale vocabulary: %s", encoded)
	}
	got, err := DecodePrompt(raw)
	if err != nil {
		t.Fatalf("DecodePrompt: %v", err)
	}
	if got.Kind != interrupt.Question || got.Question == nil || got.Question.ToolName != "ask_user" ||
		!got.Question.Fields[0].AllowCustom {
		t.Fatalf("decoded = %#v", got)
	}

	approval := runs.Interrupt{
		Kind: interrupt.Approval,
		Approval: &runs.ApprovalPrompt{
			ToolName: "webfetch", Arguments: `{"url":"https://example.com"}`,
			SafetyClass: tool.SafetyClassNetwork, Risk: tool.RiskHigh,
		},
	}
	raw, err = json.Marshal(promptWireFrom(approval))
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
		[]byte(`{"kind":"approval","approval":{"toolName":"shell","arguments":"{}","safetyClass":"exec"},"question":{"toolName":"ask_user","arguments":"{}","fields":[{"prompt":"x"}]}}`),
		[]byte(`{"kind":"question","question":{"toolName":"ask_user","arguments":"{}","fields":[]}}`),
		[]byte(`{"kind":"question","question":{"toolName":"ask_user","arguments":"{}","questions":[{"question":"x"}]}}`),
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
	raw, err := EncodeResolution(interrupt.Resolution{
		Approved: true, Arguments: `{"command":"go test","description":"Run tests"}`, Answers: [][]string{{"yes"}},
		RememberScope: approval.ScopeSession,
	})
	if err != nil {
		t.Fatalf("EncodeResolution: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("decode encoded response: %v", err)
	}
	if wire["approved"] != true || wire["remember_scope"] != "session" || wire["answers"] == nil {
		t.Fatalf("response wire = %#v", wire)
	}
	if _, found := wire["Approved"]; found {
		t.Fatalf("response leaked Go field name: %#v", wire)
	}
	decoded, err := DecodeResolution(raw)
	if err != nil || decoded.RememberScope != approval.ScopeSession ||
		len(decoded.Answers) != 1 || len(decoded.Answers[0]) != 1 || decoded.Answers[0][0] != "yes" {
		t.Fatalf("DecodeResolution = %#v, %v", decoded, err)
	}
}
