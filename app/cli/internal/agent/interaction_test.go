package agent

import "testing"

func TestQuestionAnswerUsesFieldOrder(t *testing.T) {
	question := Question{RunID: "run_1", ItemID: "q_1", Title: "Configuration", Fields: []QuestionField{
		{Prompt: "Name", Kind: QuestionText},
		{Prompt: "Targets", Kind: QuestionMulti, Options: []QuestionOption{{Label: "linux"}, {Label: "darwin"}}},
	}}
	answer := QuestionAnswer{Values: [][]string{{"lyra"}, {"linux", "darwin"}}}
	if err := ValidateAnswer(question, answer); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAnswer(question, QuestionAnswer{Values: [][]string{{"lyra"}}}); err == nil {
		t.Fatal("partial answer set was accepted")
	}
}

func TestQuestionAcceptReturnsAnOwnedDurableFact(t *testing.T) {
	t.Parallel()
	question := Question{RunID: "run_1", ItemID: "q_1", Title: "Target", Fields: []QuestionField{{Prompt: "Name", Kind: QuestionText}}}
	answer := QuestionAnswer{Values: [][]string{{"linux"}}}
	accepted, err := question.Accept(answer)
	if err != nil {
		t.Fatal(err)
	}
	if question.Answered() || !accepted.Answered() || accepted.Answers[0][0] != "linux" {
		t.Fatalf("pending = %+v, accepted = %+v", question, accepted)
	}
	answer.Values[0][0] = "mutated"
	if accepted.Answers[0][0] != "linux" {
		t.Fatal("accepted question aliases the editor's response storage")
	}
	if _, err := accepted.Accept(QuestionAnswer{Values: [][]string{{"again"}}}); err == nil {
		t.Fatal("accepted question accepted a second response")
	}
}

func TestApprovalHonorsRememberable(t *testing.T) {
	approval := runningApproval("a_1", "shell")
	if err := ValidateAnswer(approval, ApprovalAnswer{Decision: ApprovalApprove, Remember: RememberProject}); err == nil {
		t.Fatal("unrememberable approval was remembered")
	}
	approval.Rememberable = true
	if err := ValidateAnswer(approval, ApprovalAnswer{Decision: ApprovalApprove, Remember: RememberProject}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAnswer(approval, ApprovalAnswer{Decision: ApprovalDeny, Remember: RememberSession}); err != nil {
		t.Fatalf("remembered denial was rejected: %v", err)
	}
}

func TestApprovalAnswerOwnsOnlyApprovedArgumentOverrides(t *testing.T) {
	t.Parallel()
	approval := runningApproval("a_1", "shell")
	override, err := ParseToolArgumentOverride([]byte(`{"command":"go test ./..."}`))
	if err != nil {
		t.Fatal(err)
	}
	approved := ApprovalAnswer{Decision: ApprovalApprove, ArgumentOverride: override}
	if err := ValidateAnswer(approval, approved); err != nil {
		t.Fatal(err)
	}
	cloned := CloneAnswer(approved).(ApprovalAnswer)
	if cloned.ArgumentOverride == approved.ArgumentOverride || !cloned.ArgumentOverride.Equal(approved.ArgumentOverride) {
		t.Fatal("approval answer did not deep-clone its argument override")
	}
	denied := approved
	denied.Decision = ApprovalDeny
	if err := ValidateAnswer(approval, denied); err == nil {
		t.Fatal("denied approval retained an argument override")
	}
}

func runningApproval(itemID, title string) Approval {
	return Approval{
		RunID: "run_1", ItemID: itemID, Title: title,
		Tool: &ToolCall{Kind: ToolShell, Name: "shell", Status: ToolRunning},
	}
}

func TestQuestionFieldEnforcesRuntimePresentationBounds(t *testing.T) {
	if err := (QuestionField{Prompt: "Choose", Header: "1234567890123", Kind: QuestionText}).Validate(); err == nil {
		t.Fatal("overlong question header was accepted")
	}
	if err := (QuestionField{Prompt: "Choose", Kind: QuestionSingle, Options: []QuestionOption{{Label: "only"}}}).Validate(); err == nil {
		t.Fatal("single-option choice was accepted")
	}
}

func TestCompletedQuestionOwnsAndValidatesAcceptedAnswers(t *testing.T) {
	t.Parallel()

	question := Question{
		RunID: "run_1", ItemID: "q_1", Title: "Configuration",
		Fields: []QuestionField{
			{Prompt: "Target", Kind: QuestionText},
			{Prompt: "Checks", Kind: QuestionMulti, Options: []QuestionOption{{Label: "unit"}, {Label: "smoke"}}},
		},
		Answers: [][]string{{"linux"}, {"unit", "smoke"}},
	}
	if err := question.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := ValidateInteraction(question); err == nil {
		t.Fatal("answered transcript question was accepted as a pending interaction")
	}
	cloned := question.Clone()
	if !question.Equal(cloned) {
		t.Fatal("question clone changed its value")
	}
	pending := question.Clone()
	pending.Answers = nil
	if question.Equal(pending) {
		t.Fatal("completed question equals its pending form")
	}
	cloned.Answers[0][0] = "mutated"
	if question.Answers[0][0] == "mutated" || question.Equal(cloned) {
		t.Fatal("question clone shares accepted answer storage")
	}

	invalid := question.Clone()
	invalid.Answers[1] = []string{"integration"}
	if err := invalid.Validate(); err == nil {
		t.Fatal("question accepted an answer outside its declared options")
	}
}
