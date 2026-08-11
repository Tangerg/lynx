package terminal

import (
	"slices"
	"testing"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func TestQuestionnaireOwnsAnswersAndNavigation(t *testing.T) {
	question := agent.Question{
		ItemID: "plan", Title: "Plan deployment",
		Fields: []agent.QuestionField{
			{Prompt: "Goal", Kind: agent.QuestionText},
			{Prompt: "Strategy", Kind: agent.QuestionSingle, Options: []agent.QuestionOption{{Label: "Safe"}, {Label: "Fast"}}},
			{Prompt: "Checks", Kind: agent.QuestionMulti, Options: []agent.QuestionOption{{Label: "Unit"}, {Label: "Integration"}}},
		},
	}
	previous := agent.QuestionAnswer{Values: [][]string{{"release"}, {"Fast"}, {"Unit"}}}
	review, err := newQuestionnaire(question, previous)
	if err != nil {
		t.Fatal(err)
	}
	question.Fields[0].Prompt = "mutated by caller"
	previous.Values[0][0] = "mutated by caller"
	if review.Title() != "Plan deployment · 1/3" || *review.Text(0) != "release" {
		t.Fatalf("initial questionnaire = %q, %q", review.Title(), *review.Text(0))
	}
	if !review.Advance() || review.Title() != "Plan deployment · 2/3" || !review.Advance() || review.Advance() {
		t.Fatalf("forward navigation stopped at %q", review.Title())
	}
	if !review.Back() || review.Title() != "Plan deployment · 2/3" {
		t.Fatalf("back navigation stopped at %q", review.Title())
	}

	answer, err := review.Answer()
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"release"}, {"Fast"}, {"Unit"}}
	if !slices.EqualFunc(answer.Values, want, slices.Equal) {
		t.Fatalf("answer = %+v, want %+v", answer.Values, want)
	}
}

func TestQuestionnaireNormalizesCustomMultipleValues(t *testing.T) {
	question := agent.Question{
		ItemID: "targets", Title: "Targets",
		Fields: []agent.QuestionField{{
			Prompt: "Targets", Kind: agent.QuestionMulti, AllowCustom: true,
			Options: []agent.QuestionOption{{Label: "linux"}, {Label: "darwin"}},
		}},
	}
	review, err := newQuestionnaire(question, nil)
	if err != nil {
		t.Fatal(err)
	}
	*review.Text(0) = " linux, custom ,darwin "
	answer, err := review.Answer()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"linux", "custom", "darwin"}
	if !slices.Equal(answer.Values[0], want) {
		t.Fatalf("custom values = %q, want %q", answer.Values[0], want)
	}
}

func TestCustomMultipleValuesRejectEmptyAndDuplicateInput(t *testing.T) {
	for _, test := range []struct {
		name  string
		value string
	}{
		{name: "empty separators", value: " , , "},
		{name: "duplicate", value: "linux, darwin, linux"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseCustomChoices(test.value); err == nil {
				t.Fatalf("parseCustomChoices(%q) accepted invalid input", test.value)
			}
		})
	}
}

func TestQuestionnaireRejectsMissingFieldsAndIncompleteAnswers(t *testing.T) {
	if _, err := newQuestionnaire(agent.Question{}, nil); err == nil {
		t.Fatal("question without fields was accepted")
	}
	question := agent.Question{
		ItemID: "goal", Title: "Goal",
		Fields: []agent.QuestionField{{Prompt: "Goal", Kind: agent.QuestionText}},
	}
	review, err := newQuestionnaire(question, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := review.Answer(); err == nil {
		t.Fatal("empty required answer was accepted")
	}
}
