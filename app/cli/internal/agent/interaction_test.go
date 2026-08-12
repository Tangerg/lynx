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

func TestApprovalHonorsRememberable(t *testing.T) {
	approval := runningApproval("a_1", "shell")
	if err := ValidateAnswer(approval, ApprovalAnswer{Decision: ApprovalApprove, Remember: RememberProject}); err == nil {
		t.Fatal("unrememberable approval was remembered")
	}
	approval.Rememberable = true
	if err := ValidateAnswer(approval, ApprovalAnswer{Decision: ApprovalApprove, Remember: RememberProject}); err != nil {
		t.Fatal(err)
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
