package agent

import "testing"

func TestResumeRunEqualityIncludesCompleteDecisionValue(t *testing.T) {
	override, err := ParseToolArgumentOverride([]byte(`{"command":"go test ./..."}`))
	if err != nil {
		t.Fatal(err)
	}
	message := Message{Text: "continue after approval"}
	resume := ResumeRun{
		CommandID: CommandID("cli_77777777777777777777777777777777"), RunID: "run_1", Message: &message,
		Answers: []InterruptAnswer{
			{ItemID: "approval", Answer: ApprovalAnswer{Decision: ApprovalApprove, ArgumentOverride: override}},
			{ItemID: "question", Answer: QuestionAnswer{Values: [][]string{{"linux", "darwin"}}}},
		},
	}
	if !resume.Equal(resume.Clone()) {
		t.Fatal("cloned resume command is not equal")
	}

	changed := resume.Clone()
	changed.Answers[0].Answer = ApprovalAnswer{Decision: ApprovalDeny, Reason: "unsafe"}
	if resume.Equal(changed) {
		t.Fatal("resume equality ignored an approval decision")
	}
	changed = resume.Clone()
	changed.Answers[1].Answer.(QuestionAnswer).Values[0][0] = "freebsd"
	if resume.Equal(changed) {
		t.Fatal("resume equality ignored nested question values")
	}
	changed = resume.Clone()
	changed.Message.Text = "different guidance"
	if resume.Equal(changed) {
		t.Fatal("resume equality ignored its message")
	}
}
