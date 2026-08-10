package client

import (
	"strings"
	"testing"
)

func TestQuestionValidationRejectsAmbiguousSchemas(t *testing.T) {
	question := Question{
		InterruptID: "question_1",
		Title:       "Choose",
		Fields: []QuestionField{
			{ID: "strategy", Label: "Strategy", Kind: QuestionSingle, Options: []QuestionOption{{Value: "safe", Recommended: true}, {Value: "safe", Recommended: true}}},
			{ID: "strategy", Label: "Again", Kind: QuestionText},
		},
	}
	err := question.Validate()
	if err == nil {
		t.Fatal("ambiguous question was accepted")
	}
	for _, want := range []string{"repeats option", "duplicated"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("validation error %q does not mention %q", err, want)
		}
	}
}

func TestValidateAnswerEnforcesEveryQuestionKind(t *testing.T) {
	question := Question{
		InterruptID: "question_1",
		Title:       "Configure",
		Fields: []QuestionField{
			{ID: "name", Label: "Name", Kind: QuestionText, Required: true},
			{ID: "strategy", Label: "Strategy", Kind: QuestionSingle, Options: []QuestionOption{{Value: "safe"}, {Value: "fast"}}},
			{ID: "checks", Label: "Checks", Kind: QuestionMulti, Options: []QuestionOption{{Value: "test"}, {Value: "lint"}}},
			{ID: "confirm", Label: "Confirm", Kind: QuestionBool},
		},
	}
	valid := QuestionAnswer{Values: map[string][]string{
		"name": {"cache"}, "strategy": {"safe"}, "checks": {"test", "lint"}, "confirm": {"false"},
	}}
	if err := ValidateAnswer(question, valid); err != nil {
		t.Fatalf("valid answer: %v", err)
	}
	for name, answer := range map[string]QuestionAnswer{
		"required":   {Values: map[string][]string{"name": {" "}}},
		"unknown":    {Values: map[string][]string{"name": {"cache"}, "extra": {"x"}}},
		"single":     {Values: map[string][]string{"name": {"cache"}, "strategy": {"safe", "fast"}}},
		"multi":      {Values: map[string][]string{"name": {"cache"}, "checks": {"test", "test"}}},
		"boolean":    {Values: map[string][]string{"name": {"cache"}, "confirm": {"maybe"}}},
		"cancel mix": {Canceled: true, Values: map[string][]string{"name": {"cache"}}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateAnswer(question, answer); err == nil {
				t.Fatal("invalid answer was accepted")
			}
		})
	}
}

func TestApprovalAnswerCannotRememberADenial(t *testing.T) {
	approval := Approval{InterruptID: "approval_1", Title: "Edit file"}
	if err := ValidateAnswer(approval, ApprovalAnswer{Decision: ApprovalAllow, Remember: RememberProject}); err != nil {
		t.Fatalf("valid approval answer: %v", err)
	}
	if err := ValidateAnswer(approval, ApprovalAnswer{Decision: ApprovalDeny, Remember: RememberGlobal}); err == nil {
		t.Fatal("remembered denial was accepted as an allow rule")
	}
}

func TestAnswerAndInteractionClonesOwnNestedCollections(t *testing.T) {
	question := Question{InterruptID: "q", Title: "Q", Fields: []QuestionField{{ID: "f", Label: "F", Kind: QuestionSingle, Options: []QuestionOption{{Value: "a"}}}}}
	clonedQuestion := CloneInteraction(question).(Question)
	clonedQuestion.Fields[0].Options[0].Value = "changed"
	if question.Fields[0].Options[0].Value != "a" {
		t.Fatal("CloneInteraction shared question options")
	}

	answer := QuestionAnswer{Values: map[string][]string{"f": {"a"}}}
	clonedAnswer := CloneAnswer(answer).(QuestionAnswer)
	clonedAnswer.Values["f"][0] = "changed"
	if answer.Values["f"][0] != "a" || EqualAnswers(answer, clonedAnswer) {
		t.Fatal("CloneAnswer shared values or EqualAnswers ignored a change")
	}
}
