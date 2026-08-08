package askuser

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
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
	for _, arguments := range []string{
		`{"questions":[{"question":""}]}`,
		`{"questions":[{"question":"Choose","header":"1234567890123"}]}`,
		`{"questions":[{"question":"Choose","options":[{"label":"one"}]}]}`,
		`{"questions":[{"question":"Choose","options":[{"label":""},{"label":"two"}]}]}`,
		`{"questions":[{"question":"Choose","multi_select":true}]}`,
	} {
		if _, err := tool.Call(context.Background(), arguments); err == nil {
			t.Errorf("arguments outside the ask_user contract must error: %s", arguments)
		}
	}
}

func TestAskUserKeepsOptionsOpenToARealUserAnswer(t *testing.T) {
	var captured runs.Interrupt
	tool, err := New(func(_ context.Context, _ string, request runs.Interrupt) (interrupt.Resolution, error) {
		captured = request
		return interrupt.Resolution{Answers: [][]string{{"another database"}}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := tool.Call(context.Background(), `{
		"questions":[{
			"question":"Pick a database",
			"options":[{"label":"Postgres"},{"label":"SQLite"}]
		}]
	}`)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result != "another database" {
		t.Fatalf("result = %q", result)
	}
	if captured.Question == nil || len(captured.Question.Fields) != 1 ||
		!captured.Question.Fields[0].AllowCustom {
		t.Fatalf("question interrupt = %#v, want custom answer enabled", captured.Question)
	}
}

// TestAnswerText covers the result rendering: a single question returns just its
// answer; multiple questions return labeled lines; multi-select joins values.
func TestAnswerText(t *testing.T) {
	single := runs.QuestionPrompt{Fields: []runs.QuestionFieldSpec{{Prompt: "Proceed?"}}}
	if got := answerText(single, [][]string{{"yes"}}); got != "yes" {
		t.Errorf("single = %q, want %q", got, "yes")
	}

	multi := runs.QuestionPrompt{Fields: []runs.QuestionFieldSpec{
		{Prompt: "Pick a DB", Header: "DB"},
		{Prompt: "Pick langs", Header: "Langs", Multiple: true},
	}}
	answers := [][]string{{"sqlite"}, {"go", "rust"}}
	got := answerText(multi, answers)
	if !strings.Contains(got, "DB: sqlite") || !strings.Contains(got, "Langs: go, rust") {
		t.Errorf("multi = %q, want labeled lines incl. \"DB: sqlite\" and \"Langs: go, rust\"", got)
	}
}
