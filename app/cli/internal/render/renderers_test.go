package render

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func TestTextRendersStreamedAnswerToolAndUsage(t *testing.T) {
	var output bytes.Buffer
	renderer := NewText(&output)
	for _, event := range testEvents() {
		if err := renderer.Render(event); err != nil {
			t.Fatal(err)
		}
	}
	if err := renderer.Close(); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, want := range []string{"hello world", "go test ./...", "PASS", "↑ 1,200", "cached 600"} {
		if !strings.Contains(text, want) {
			t.Fatalf("text output does not contain %q:\n%s", want, text)
		}
	}
}

func TestNDJSONCarriesSegmentIdentityAndInterruptSet(t *testing.T) {
	var output bytes.Buffer
	renderer := NewNDJSON(&output)
	events := []agent.RunEvent{
		testEvent("evt_start", agent.SegmentStarted{Run: testRun()}),
		testEvent("evt_wait", agent.RunInterrupted{Usage: agent.Usage{InputTokens: 42}, Interactions: []agent.Interaction{
			testApproval("tool_1", "shell"),
			agent.Question{ItemID: "question_1", Title: "choose", Fields: []agent.QuestionField{{Prompt: "Target", Kind: agent.QuestionSingle, Options: []agent.QuestionOption{{Label: "linux"}, {Label: "darwin"}}}}},
		}}),
	}
	for _, event := range events {
		if err := renderer.Render(event); err != nil {
			t.Fatal(err)
		}
	}
	if err := renderer.Close(); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("NDJSON lines = %d", len(lines))
	}
	var frame map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &frame); err != nil {
		t.Fatal(err)
	}
	if frame["segmentId"] != "seg_1" || frame["eventId"] != "evt_wait" {
		t.Fatalf("event identity = %+v", frame)
	}
	interactions, ok := frame["interactions"].([]any)
	if !ok || len(interactions) != 2 {
		t.Fatalf("interactions = %#v", frame["interactions"])
	}
	usage, ok := frame["usage"].(map[string]any)
	if !ok || usage["inputTokens"] != float64(42) {
		t.Fatalf("interrupt usage = %#v", frame["usage"])
	}
}

func TestResultJSONClearsPriorInterruptWhenANewSegmentStarts(t *testing.T) {
	var output bytes.Buffer
	renderer := NewResultJSON(&output)
	if err := renderer.Begin(testRun(), agent.RunOptions{}); err != nil {
		t.Fatal(err)
	}
	question := agent.Question{
		ItemID: "question_1", Title: "choose",
		Fields: []agent.QuestionField{{Prompt: "Target", Kind: agent.QuestionSingle, Options: []agent.QuestionOption{{Label: "linux"}, {Label: "darwin"}}}},
	}
	resumed := testRun()
	resumed.ActiveSegmentID = "seg_2"
	for _, event := range []agent.RunEvent{
		testEvent("start", agent.SegmentStarted{Run: testRun()}),
		testEvent("wait", agent.RunInterrupted{Interactions: []agent.Interaction{question}}),
		{EventID: "resume", RunID: "run_1", SegmentID: "seg_2", Event: agent.SegmentStarted{Run: resumed}},
	} {
		if err := renderer.Render(event); err != nil {
			t.Fatal(err)
		}
	}
	if err := renderer.Close(); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["status"] != "incomplete" || result["interactions"] != nil {
		t.Fatalf("resumed result = %+v", result)
	}
}

func TestResultJSONFoldsFinalAssistantProjection(t *testing.T) {
	var output bytes.Buffer
	renderer := NewResultJSON(&output)
	if err := renderer.Begin(testRun(), agent.RunOptions{Provider: "mock", Model: "balanced"}); err != nil {
		t.Fatal(err)
	}
	for _, event := range testEvents() {
		if err := renderer.Render(event); err != nil {
			t.Fatal(err)
		}
	}
	if err := renderer.Close(); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["status"] != "completed" || result["text"] != "hello world" || result["runId"] != "run_1" {
		t.Fatalf("result = %+v", result)
	}
}

func TestColdReconciliationKeepsOneShotOutputScopedToItsRun(t *testing.T) {
	snapshot := reconciliationSnapshot()
	requireScopedResultJSON(t, snapshot)
	requireScopedText(t, snapshot)
	requireScopedStream(t, snapshot)
}

func reconciliationSnapshot() agent.SessionSnapshot {
	return agent.SessionSnapshot{
		Session: agent.Session{ID: "ses_1", Status: agent.SessionIdle, Workspace: "/tmp/demo"},
		Transcript: []agent.Block{
			{ID: "old", RunID: "run_old", Status: agent.BlockStatusCompleted, Kind: agent.BlockAssistant, Text: "historical answer"},
			{ID: "current", RunID: "run_1", Status: agent.BlockStatusCompleted, Kind: agent.BlockAssistant, Text: "current answer"},
			{ID: "new", RunID: "run_new", Status: agent.BlockStatusCompleted, Kind: agent.BlockAssistant, Text: "newer answer"},
		},
		Runs: []agent.Run{
			{ID: "run_old", SessionID: "ses_1", Status: agent.RunStatusFinished, Outcome: agent.Outcome{Status: agent.OutcomeCompleted}},
			{ID: "run_1", SessionID: "ses_1", Status: agent.RunStatusFinished, Outcome: agent.Outcome{Status: agent.OutcomeCompleted}},
			{ID: "run_new", SessionID: "ses_1", Status: agent.RunStatusFinished, Outcome: agent.Outcome{Status: agent.OutcomeCompleted}},
		},
		Plan: []agent.PlanItem{{Title: "newer plan", Status: agent.PlanDone}}, PlanRevision: 3,
	}
}

func requireScopedResultJSON(t *testing.T, snapshot agent.SessionSnapshot) {
	t.Helper()
	var output bytes.Buffer
	renderer := NewResultJSON(&output)
	if err := renderer.Begin(testRun(), agent.RunOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := renderer.Reconcile(snapshot); err != nil {
		t.Fatal(err)
	}
	if err := renderer.Close(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "historical answer") || strings.Contains(output.String(), "newer answer") || !strings.Contains(output.String(), "current answer") {
		t.Fatalf("reconciled result = %s", output.String())
	}
}

func requireScopedText(t *testing.T, snapshot agent.SessionSnapshot) {
	t.Helper()
	var output bytes.Buffer
	textRenderer := NewText(&output)
	if err := textRenderer.Begin(testRun(), agent.RunOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := textRenderer.Reconcile(snapshot); err != nil {
		t.Fatal(err)
	}
	if err := textRenderer.Close(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "historical answer") || strings.Contains(output.String(), "newer answer") || !strings.Contains(output.String(), "current answer") {
		t.Fatalf("reconciled text = %s", output.String())
	}
}

func requireScopedStream(t *testing.T, snapshot agent.SessionSnapshot) {
	t.Helper()
	var output bytes.Buffer
	streamRenderer := NewNDJSON(&output)
	if err := streamRenderer.Begin(testRun(), agent.RunOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := streamRenderer.Reconcile(snapshot); err != nil {
		t.Fatal(err)
	}
	var frame eventRecord
	if err := json.Unmarshal(output.Bytes(), &frame); err != nil {
		t.Fatal(err)
	}
	if frame.RunID != "run_1" || len(frame.Transcript) != 1 || frame.Transcript[0].Text != "current answer" {
		t.Fatalf("reconciled stream = %+v", frame)
	}
	if frame.Revision != 0 || len(frame.Plan) != 0 {
		t.Fatalf("reconciled stream leaked newer plan = %+v", frame.Plan)
	}
}

func TestRenderersRejectInvalidEvents(t *testing.T) {
	invalid := agent.RunEvent{EventID: "evt", RunID: "run_1", SegmentID: "seg_1", Event: agent.BlockDelta{}}
	var output bytes.Buffer
	if err := NewText(&output).Render(invalid); err == nil {
		t.Fatal("text accepted invalid event")
	}
	if err := NewNDJSON(&output).Render(invalid); err == nil {
		t.Fatal("NDJSON accepted invalid event")
	}
	if err := NewResultJSON(&output).Render(invalid); err == nil {
		t.Fatal("result JSON accepted invalid event")
	}
}

func TestUsageJSONDistinguishesUnknownFromKnownZeroCost(t *testing.T) {
	unknown, err := json.Marshal(encodeUsage(agent.Usage{}))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(unknown, []byte(`"costUsd"`)) {
		t.Fatalf("unknown cost was serialized: %s", unknown)
	}
	known, err := json.Marshal(encodeUsage(agent.Usage{CostUSD: new(0.0)}))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(known, []byte(`"costUsd":0`)) {
		t.Fatalf("known zero cost was omitted: %s", known)
	}
}

func testEvents() []agent.RunEvent {
	code := 0
	return []agent.RunEvent{
		testEvent("evt_start", agent.SegmentStarted{Run: testRun()}),
		testEvent("evt_message_start", agent.BlockStarted{Block: agent.Block{ID: "msg_1", Kind: agent.BlockAssistant}}),
		testEvent("evt_message_delta_1", agent.BlockDelta{BlockID: "msg_1", Text: "hello "}),
		testEvent("evt_message_delta_2", agent.BlockDelta{BlockID: "msg_1", Text: "world"}),
		testEvent("evt_message_done", agent.BlockCompleted{Block: agent.Block{ID: "msg_1", Kind: agent.BlockAssistant, Text: "hello world"}}),
		testEvent("evt_tool", agent.BlockCompleted{Block: agent.Block{ID: "tool_1", Kind: agent.BlockTool, Tool: &agent.ToolCall{
			Kind: agent.ToolShell, Name: "shell", Summary: "go test ./...", Status: agent.ToolOK,
			Command: "go test ./...", Output: "PASS", ExitCode: &code,
		}}}),
		testEvent("evt_done", agent.RunFinished{
			Outcome: agent.Outcome{Status: agent.OutcomeCompleted},
			Usage:   agent.Usage{InputTokens: 1_200, OutputTokens: 80, CacheReadTokens: 600, CostUSD: new(0.01), Duration: time.Second},
		}),
	}
}

func testEvent(id string, event agent.Event) agent.RunEvent {
	switch item := event.(type) {
	case agent.BlockStarted:
		item.Block.RunID = "run_1"
		item.Block.Status = agent.BlockStatusRunning
		event = item
	case agent.BlockCompleted:
		item.Block.RunID = "run_1"
		item.Block.Status = agent.BlockStatusCompleted
		event = item
	}
	return agent.RunEvent{EventID: id, RunID: "run_1", SegmentID: "seg_1", At: time.Unix(1, 0), Event: event}
}

func testRun() agent.Run {
	return agent.Run{ID: "run_1", SessionID: "ses_1", Provider: "mock", Model: "balanced", Status: agent.RunStatusRunning, ActiveSegmentID: "seg_1"}
}

func testApproval(itemID, title string) agent.Approval {
	return agent.Approval{
		ItemID: itemID, Title: title,
		Tool: &agent.ToolCall{Kind: agent.ToolShell, Name: "shell", Status: agent.ToolRunning},
	}
}
