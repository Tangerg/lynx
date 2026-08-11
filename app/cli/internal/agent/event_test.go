package agent

import (
	"testing"
	"time"
)

func TestRunEventEqualityUsesDomainValues(t *testing.T) {
	t.Parallel()
	cost := 0.0
	approval := Approval{
		ItemID: "approval_1", Title: "Run command", Rememberable: true,
		Tool: &ToolCall{Kind: ToolShell, Name: "shell", Status: ToolRunning, Command: "go test ./..."},
	}
	question := Question{
		ItemID: "question_1", Title: "Choose target",
		Fields: []QuestionField{{
			Prompt: "Target", Kind: QuestionSingle,
			Options: []QuestionOption{{Label: "linux"}, {Label: "darwin"}},
		}},
	}
	at := time.Date(2026, time.August, 12, 8, 0, 0, 0, time.UTC)
	event := RunEvent{
		EventID: "event_1", RunID: "run_1", SegmentID: "segment_1", At: at,
		Event: RunInterrupted{
			Interactions: []Interaction{approval, question},
			Usage:        Usage{InputTokens: 3, CostUSD: &cost, Duration: time.Second},
		},
	}

	clone := event.Clone()
	clone.At = at.In(time.FixedZone("same-instant", 8*60*60))
	if !event.Equal(clone) {
		t.Fatal("a cloned event at the same instant is not equal")
	}

	changed := event.Clone()
	interrupted := changed.Event.(RunInterrupted)
	changedApproval := interrupted.Interactions[0].(Approval)
	changedApproval.Tool.Output = "different"
	interrupted.Interactions[0] = changedApproval
	changed.Event = interrupted
	if event.Equal(changed) {
		t.Fatal("a nested tool projection change was ignored")
	}
	changed = event.Clone()
	interrupted = changed.Event.(RunInterrupted)
	changedQuestion := interrupted.Interactions[1].(Question)
	changedQuestion.Fields[0].Options[0].Description = "different"
	interrupted.Interactions[1] = changedQuestion
	changed.Event = interrupted
	if event.Equal(changed) {
		t.Fatal("a nested question option change was ignored")
	}

	unknownCost := event.Clone()
	interrupted = unknownCost.Event.(RunInterrupted)
	interrupted.Usage.CostUSD = nil
	unknownCost.Event = interrupted
	if event.Equal(unknownCost) {
		t.Fatal("unknown cost was treated as an explicit zero cost")
	}
}
