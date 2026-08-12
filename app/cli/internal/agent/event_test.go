package agent

import (
	"bytes"
	"testing"
	"time"
)

func TestRunEventEqualityUsesDomainValues(t *testing.T) {
	t.Parallel()
	cost := 0.0
	approval := Approval{
		RunID: "run_1", ItemID: "approval_1", Title: "Run command", Rememberable: true,
		Tool: &ToolCall{Kind: ToolShell, Name: "shell", Status: ToolRunning, Command: "go test ./..."},
	}
	question := Question{
		RunID: "run_1", ItemID: "question_1", Title: "Choose target",
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

func TestEphemeralEventsCloneOwnedValues(t *testing.T) {
	step, contextTokens, cost := 2, int64(4_096), 0.5
	progress := RunProgress{
		Step: &step, ContextTokens: &contextTokens,
		Usage: &Usage{InputTokens: 10, CostUSD: &cost}, Activity: "calling tools",
	}
	clone := CloneEvent(progress).(RunProgress)
	*clone.Step, *clone.ContextTokens, clone.Usage.InputTokens, *clone.Usage.CostUSD = 9, 1, 99, 9
	if *progress.Step != 2 || *progress.ContextTokens != 4_096 || progress.Usage.InputTokens != 10 || *progress.Usage.CostUSD != 0.5 {
		t.Fatalf("progress clone aliases source: source=%+v clone=%+v", progress, clone)
	}

	custom := CustomEvent{Name: "vendor.trace", PayloadJSON: []byte(`{"span":"abc"}`)}
	customClone := CloneEvent(custom).(CustomEvent)
	customClone.PayloadJSON[0] = '['
	if !bytes.Equal(custom.PayloadJSON, []byte(`{"span":"abc"}`)) {
		t.Fatalf("custom clone aliases source: %s", custom.PayloadJSON)
	}
}

func TestEphemeralEventValidationRejectsMalformedValues(t *testing.T) {
	negative := -1
	negativeContext := int64(-1)
	tests := []Event{
		RunProgress{Step: &negative},
		RunProgress{ContextTokens: &negativeContext},
		ToolArgumentsDelta{},
		CustomEvent{Name: "vendor.trace", PayloadJSON: []byte(`{`)},
		CustomEvent{PayloadJSON: []byte(`null`)},
	}
	for _, event := range tests {
		if err := ValidateEvent(event); err == nil {
			t.Fatalf("ValidateEvent(%T) accepted %+v", event, event)
		}
	}
}

func TestBlockCloneOwnsAssistantImageBytes(t *testing.T) {
	block := Block{
		ID: "answer", RunID: "run_1", Status: BlockStatusCompleted, Kind: BlockAssistant,
		Images: []InlineImage{{ID: "answer:image:0", Name: "chart.png", MIMEType: "image/png", Data: []byte("png")}},
	}
	if err := ValidateEvent(BlockCompleted{Block: block}); err != nil {
		t.Fatal(err)
	}
	clone := block.Clone()
	clone.Images[0].Data[0] = 'x'
	if bytes.Equal(block.Images[0].Data, clone.Images[0].Data) || !block.Equal(block.Clone()) {
		t.Fatalf("inline image clone is not value-owned: source=%+v clone=%+v", block.Images, clone.Images)
	}
}

func TestToolCallCloneOwnsRawJSON(t *testing.T) {
	call := ToolCall{
		Kind: ToolUnknown, Name: "provider_tool", Status: ToolOK,
		ArgumentsJSON: []byte(`{"nested":{"value":1}}`), ResultJSON: []byte(`{"ok":true}`),
	}
	if err := call.Validate(); err != nil {
		t.Fatal(err)
	}
	clone := call.Clone()
	clone.ArgumentsJSON[0], clone.ResultJSON[0] = '[', '['
	if bytes.Equal(call.ArgumentsJSON, clone.ArgumentsJSON) || bytes.Equal(call.ResultJSON, clone.ResultJSON) || !call.Equal(call.Clone()) {
		t.Fatalf("tool JSON clone aliases source: source=%+v clone=%+v", call, clone)
	}
}
