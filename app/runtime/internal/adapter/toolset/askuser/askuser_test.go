package askuser

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
)

// TestAskUser_Validation: malformed args and an empty questions list are
// model-facing errors raised before the call parks (no HITL context needed).
func TestAskUser_Validation(t *testing.T) {
	tool, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tool.Call(context.Background(), `not json`); err == nil {
		t.Error("invalid JSON must error")
	}
	if _, err := tool.Call(context.Background(), `{"questions":[]}`); err == nil {
		t.Error("empty questions must error")
	}
}

// TestAnswerText covers the result rendering: a single question returns just its
// answer; multiple questions return labeled lines; multi-select joins values.
func TestAnswerText(t *testing.T) {
	single := runs.QuestionPrompt{Questions: []runs.QuestionSpec{{Question: "Proceed?"}}}
	if got := answerText(single, map[string][]string{interrupts.QuestionFieldName(0): {"yes"}}); got != "yes" {
		t.Errorf("single = %q, want %q", got, "yes")
	}

	multi := runs.QuestionPrompt{Questions: []runs.QuestionSpec{
		{Question: "Pick a DB", Header: "DB"},
		{Question: "Pick langs", Header: "Langs", MultiSelect: true},
	}}
	answers := map[string][]string{
		interrupts.QuestionFieldName(0): {"sqlite"},
		interrupts.QuestionFieldName(1): {"go", "rust"},
	}
	got := answerText(multi, answers)
	if !strings.Contains(got, "DB: sqlite") || !strings.Contains(got, "Langs: go, rust") {
		t.Errorf("multi = %q, want labeled lines incl. \"DB: sqlite\" and \"Langs: go, rust\"", got)
	}
}
