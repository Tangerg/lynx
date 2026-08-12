package terminal

import (
	"testing"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func TestInteractionReviewRecordsEditsAndCommitsInRuntimeOrder(t *testing.T) {
	approval := agent.Approval{
		RunID: "run_1", ItemID: "approval", Title: "Run command", Rememberable: true,
		Tool: &agent.ToolCall{Kind: agent.ToolShell, Name: "shell", Command: "go test ./...", Status: agent.ToolRunning},
	}
	question := agent.Question{
		RunID: "run_1", ItemID: "question", Title: "Choose target",
		Fields: []agent.QuestionField{{Prompt: "Target", Kind: agent.QuestionSingle, Options: []agent.QuestionOption{{Label: "linux"}, {Label: "darwin"}}}},
	}
	review, err := newInteractionReview([]agent.Interaction{approval, question})
	if err != nil {
		t.Fatal(err)
	}
	if err := review.Record(agent.ApprovalAnswer{Decision: agent.ApprovalApprove, Remember: agent.RememberSession}); err != nil {
		t.Fatal(err)
	}
	if !review.Advance() {
		t.Fatal("review did not advance to the question")
	}
	if err := review.Record(agent.QuestionAnswer{Values: [][]string{{"linux"}}}); err != nil {
		t.Fatal(err)
	}
	if review.Advance() || !review.Reviewing() {
		t.Fatal("review did not enter final review")
	}
	if !review.Back() {
		t.Fatal("review did not return to the final item")
	}
	if err := review.Record(agent.QuestionAnswer{Values: [][]string{{"darwin"}}}); err != nil {
		t.Fatal(err)
	}
	review.Advance()
	responses, err := review.Responses()
	if err != nil {
		t.Fatal(err)
	}
	if len(responses) != 2 || responses[0].ItemID != "approval" || responses[1].ItemID != "question" {
		t.Fatalf("responses = %+v", responses)
	}
	answer, ok := responses[1].Answer.(agent.QuestionAnswer)
	if !ok || answer.Values[0][0] != "darwin" {
		t.Fatalf("edited question answer = %#v", responses[1].Answer)
	}
}

func TestInteractionReviewRejectsInvalidAnswersAndIncompleteCommit(t *testing.T) {
	approval := agent.Approval{
		RunID: "run_1", ItemID: "approval", Title: "Read file",
		Tool: &agent.ToolCall{Kind: agent.ToolRead, Name: "read", Path: "README.md", Status: agent.ToolRunning},
	}
	review, err := newInteractionReview([]agent.Interaction{approval})
	if err != nil {
		t.Fatal(err)
	}
	if err := review.Record(agent.QuestionAnswer{}); err == nil {
		t.Fatal("invalid answer was accepted")
	}
	if _, err := review.Responses(); err == nil {
		t.Fatal("incomplete review was committed")
	}
}

func TestInteractionReviewRestoresACommittedBatchWithoutSharingAnswers(t *testing.T) {
	approval := agent.Approval{
		RunID: "run_1", ItemID: "approval", Title: "Run command", Rememberable: true,
		Tool: &agent.ToolCall{Kind: agent.ToolShell, Name: "shell", Command: "go test ./...", Status: agent.ToolRunning},
	}
	question := agent.Question{
		RunID: "run_1", ItemID: "question", Title: "Choose target",
		Fields: []agent.QuestionField{{
			Prompt: "Target", Kind: agent.QuestionMulti,
			Options: []agent.QuestionOption{{Label: "linux"}, {Label: "darwin"}},
		}},
	}
	responses := []agent.InterruptAnswer{
		{ItemID: approval.ItemID, Answer: agent.ApprovalAnswer{Decision: agent.ApprovalApprove, Remember: agent.RememberSession}},
		{ItemID: question.ItemID, Answer: agent.QuestionAnswer{Values: [][]string{{"linux", "darwin"}}}},
	}
	review, err := restoreInteractionReview([]agent.Interaction{approval, question}, responses)
	if err != nil {
		t.Fatal(err)
	}
	if !review.Reviewing() {
		t.Fatal("restored review is not at its committed summary")
	}
	responses[1].Answer.(agent.QuestionAnswer).Values[0][0] = "mutated"
	committed, err := review.Responses()
	if err != nil {
		t.Fatal(err)
	}
	answer := committed[1].Answer.(agent.QuestionAnswer)
	if answer.Values[0][0] != "linux" {
		t.Fatalf("restored answer shares caller storage: %+v", answer.Values)
	}
}
