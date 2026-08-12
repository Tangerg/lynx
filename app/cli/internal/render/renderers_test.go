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
			agent.Question{RunID: "run_1", ItemID: "question_1", Title: "choose", Fields: []agent.QuestionField{{Prompt: "Target", Kind: agent.QuestionSingle, Options: []agent.QuestionOption{{Label: "linux"}, {Label: "darwin"}}}}},
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
	approval := interactions[0].(map[string]any)
	tool := approval["tool"].(map[string]any)
	arguments := tool["arguments"].(map[string]any)
	if approval["rememberable"] != true || tool["name"] != "shell" || arguments["command"] != "go test ./..." {
		t.Fatalf("approval interaction = %#v", approval)
	}
	usage, ok := frame["usage"].(map[string]any)
	if !ok || usage["inputTokens"] != float64(42) {
		t.Fatalf("interrupt usage = %#v", frame["usage"])
	}
}

func TestNDJSONPreservesProgressToolArgumentsAndCustomPayloads(t *testing.T) {
	var output bytes.Buffer
	renderer := NewNDJSON(&output)
	if err := renderer.Begin(testRun(), agent.RunOptions{}); err != nil {
		t.Fatal(err)
	}
	step, contextTokens := 4, int64(16_384)
	events := []agent.RunEvent{
		testEvent("progress", agent.RunProgress{Step: &step, ContextTokens: &contextTokens, Activity: "thinking", Usage: &agent.Usage{InputTokens: 22}}),
		testEvent("arguments", agent.ToolArgumentsDelta{BlockID: "tool_1", Text: `{"path":"/tmp`}),
		testEvent("custom", agent.CustomEvent{Name: "vendor.trace", PayloadJSON: []byte(`{"span":"abc","sampled":true}`)}),
	}
	for _, event := range events {
		if err := renderer.Render(event); err != nil {
			t.Fatal(err)
		}
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("NDJSON lines = %d: %s", len(lines), output.String())
	}
	var progress, arguments, custom map[string]any
	for index, target := range []*map[string]any{&progress, &arguments, &custom} {
		if err := json.Unmarshal([]byte(lines[index]), target); err != nil {
			t.Fatal(err)
		}
	}
	if progress["type"] != "run.progress" || progress["step"] != float64(step) || progress["contextTokens"] != float64(contextTokens) {
		t.Fatalf("progress frame = %+v", progress)
	}
	if arguments["type"] != "tool.arguments.delta" || arguments["blockId"] != "tool_1" || arguments["text"] != `{"path":"/tmp` {
		t.Fatalf("arguments frame = %+v", arguments)
	}
	payload, ok := custom["payload"].(map[string]any)
	if custom["type"] != "custom" || custom["name"] != "vendor.trace" || !ok || payload["span"] != "abc" {
		t.Fatalf("custom frame = %+v", custom)
	}
}

func TestResultJSONRetainsLatestRootProgressUsageBeforeSettlement(t *testing.T) {
	var output bytes.Buffer
	renderer := NewResultJSON(&output)
	if err := renderer.Begin(testRun(), agent.RunOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := renderer.Render(testEvent("progress", agent.RunProgress{Usage: &agent.Usage{InputTokens: 33, OutputTokens: 5}})); err != nil {
		t.Fatal(err)
	}
	if err := renderer.Close(); err != nil {
		t.Fatal(err)
	}
	var result resultFrame
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "incomplete" || result.Usage == nil || result.Usage.InputTokens != 33 || result.Usage.OutputTokens != 5 {
		t.Fatalf("incomplete result = %+v", result)
	}
}

func TestRenderersPreserveAssistantInlineImages(t *testing.T) {
	image := agent.InlineImage{
		ID: "answer:image:0", Name: "chart.png", MIMEType: "image/png", Data: []byte("png bytes"),
	}
	completed := testEvent("image", agent.BlockCompleted{Block: agent.Block{
		ID: "answer", Kind: agent.BlockAssistant, Text: "Generated chart", Images: []agent.InlineImage{image},
	}})

	var stream bytes.Buffer
	ndjson := NewNDJSON(&stream)
	if err := ndjson.Begin(testRun(), agent.RunOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := ndjson.Render(completed); err != nil {
		t.Fatal(err)
	}
	var event eventRecord
	if err := json.Unmarshal(stream.Bytes(), &event); err != nil {
		t.Fatal(err)
	}
	if event.Block == nil || len(event.Block.Images) != 1 || event.Block.Images[0].Name != image.Name ||
		!bytes.Equal(event.Block.Images[0].Data, image.Data) {
		t.Fatalf("stream image = %+v", event.Block)
	}

	var resultOutput bytes.Buffer
	result := NewResultJSON(&resultOutput)
	if err := result.Begin(testRun(), agent.RunOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := result.Render(completed); err != nil {
		t.Fatal(err)
	}
	if err := result.Close(); err != nil {
		t.Fatal(err)
	}
	var final resultFrame
	if err := json.Unmarshal(resultOutput.Bytes(), &final); err != nil {
		t.Fatal(err)
	}
	if len(final.Images) != 1 || !bytes.Equal(final.Images[0].Data, image.Data) {
		t.Fatalf("result images = %+v", final.Images)
	}

	var textOutput bytes.Buffer
	plain := NewText(&textOutput)
	if err := plain.Begin(testRun(), agent.RunOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := plain.Render(completed); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(textOutput.String(), "@ chart.png (image/png, 9 bytes)") {
		t.Fatalf("text image fallback = %q", textOutput.String())
	}
}

func TestNDJSONPreservesCompleteToolArgumentsAndResult(t *testing.T) {
	var output bytes.Buffer
	renderer := NewNDJSON(&output)
	if err := renderer.Begin(testRun(), agent.RunOptions{}); err != nil {
		t.Fatal(err)
	}
	event := testEvent("tool", agent.BlockCompleted{Block: agent.Block{
		ID: "tool_1", Kind: agent.BlockTool, Tool: &agent.ToolCall{
			Kind: agent.ToolUnknown, Name: "mcp__calendar__create", Status: agent.ToolOK,
			ArgumentsJSON: []byte(`{"guests":["a@example.com"]}`), ResultJSON: []byte(`{"eventId":"evt_123"}`),
		},
	}})
	if err := renderer.Render(event); err != nil {
		t.Fatal(err)
	}
	var frame map[string]any
	if err := json.Unmarshal(output.Bytes(), &frame); err != nil {
		t.Fatal(err)
	}
	block := frame["block"].(map[string]any)
	tool := block["tool"].(map[string]any)
	if tool["arguments"].(map[string]any)["guests"].([]any)[0] != "a@example.com" ||
		tool["result"].(map[string]any)["eventId"] != "evt_123" {
		t.Fatalf("tool frame = %+v", tool)
	}
}

func TestResultJSONClearsPriorInterruptWhenANewSegmentStarts(t *testing.T) {
	var output bytes.Buffer
	renderer := NewResultJSON(&output)
	if err := renderer.Begin(testRun(), agent.RunOptions{}); err != nil {
		t.Fatal(err)
	}
	question := agent.Question{
		RunID: "run_1", ItemID: "question_1", Title: "choose",
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

func TestRenderersPreserveChildRunIdentityWithoutSettlingTheRoot(t *testing.T) {
	events := runTreeEvents()

	var textOutput bytes.Buffer
	textRenderer := NewText(&textOutput)
	if err := textRenderer.Begin(testRun(), agent.RunOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if err := textRenderer.Render(event); err != nil {
			t.Fatalf("render text %s: %v", event.EventID, err)
		}
		if event.EventID == "child-finished" && textRenderer.settled {
			t.Fatal("child completion settled the root text renderer")
		}
	}
	if err := textRenderer.Close(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"child answer", "root answer"} {
		if !strings.Contains(textOutput.String(), want) {
			t.Fatalf("tree text output is missing %q:\n%s", want, textOutput.String())
		}
	}

	var streamOutput bytes.Buffer
	streamRenderer := NewNDJSON(&streamOutput)
	if err := streamRenderer.Begin(testRun(), agent.RunOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if err := streamRenderer.Render(event); err != nil {
			t.Fatalf("render NDJSON %s: %v", event.EventID, err)
		}
	}
	lines := strings.Split(strings.TrimSpace(streamOutput.String()), "\n")
	var childStart eventRecord
	if err := json.Unmarshal([]byte(lines[1]), &childStart); err != nil {
		t.Fatal(err)
	}
	if childStart.Type != "segment.started" || childStart.RunID != "run_child" ||
		childStart.ParentRunID != "run_1" || childStart.RootRunID != "run_1" ||
		childStart.SpawnedByBlockID != "spawn" || childStart.StreamSegmentID != "seg_1" || childStart.SegmentID != "seg_child" {
		t.Fatalf("child segment frame = %+v", childStart)
	}

	var resultOutput bytes.Buffer
	resultRenderer := NewResultJSON(&resultOutput)
	if err := resultRenderer.Begin(testRun(), agent.RunOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if err := resultRenderer.Render(event); err != nil {
			t.Fatalf("render result %s: %v", event.EventID, err)
		}
	}
	if err := resultRenderer.Close(); err != nil {
		t.Fatal(err)
	}
	var result resultFrame
	if err := json.Unmarshal(resultOutput.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.RunID != "run_1" || result.Status != "completed" || result.Text != "root answer" {
		t.Fatalf("tree result = %+v", result)
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

func runTreeEvents() []agent.RunEvent {
	root := testRun()
	child := agent.Run{
		ID: "run_child", SessionID: root.SessionID,
		Lineage:  agent.RunLineage{SpawnedByBlockID: "spawn", ParentRunID: root.ID, RootRunID: root.ID},
		Provider: root.Provider, Model: root.Model, Status: agent.RunStatusRunning, ActiveSegmentID: "seg_child",
	}
	event := func(id, runID, segmentID string, payload agent.Event) agent.RunEvent {
		return agent.RunEvent{
			EventID: id, RunID: runID, SegmentID: segmentID, StreamSegmentID: root.ActiveSegmentID,
			At: time.Unix(1, 0), Event: payload,
		}
	}
	block := func(runID, text string, status agent.BlockStatus) agent.Block {
		return agent.Block{ID: "answer", RunID: runID, Kind: agent.BlockAssistant, Status: status, Text: text}
	}
	return []agent.RunEvent{
		event("root-started", root.ID, root.ActiveSegmentID, agent.SegmentStarted{Run: root}),
		event("child-started", child.ID, child.ActiveSegmentID, agent.SegmentStarted{Run: child}),
		event("child-block-started", child.ID, child.ActiveSegmentID, agent.BlockStarted{Block: block(child.ID, "", agent.BlockStatusRunning)}),
		event("child-delta", child.ID, child.ActiveSegmentID, agent.BlockDelta{BlockID: "answer", Text: "child answer"}),
		event("child-block-completed", child.ID, child.ActiveSegmentID, agent.BlockCompleted{Block: block(child.ID, "child answer", agent.BlockStatusCompleted)}),
		event("child-finished", child.ID, child.ActiveSegmentID, agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}),
		event("root-block-started", root.ID, root.ActiveSegmentID, agent.BlockStarted{Block: block(root.ID, "", agent.BlockStatusRunning)}),
		event("root-delta", root.ID, root.ActiveSegmentID, agent.BlockDelta{BlockID: "answer", Text: "root answer"}),
		event("root-block-completed", root.ID, root.ActiveSegmentID, agent.BlockCompleted{Block: block(root.ID, "root answer", agent.BlockStatusCompleted)}),
		event("root-finished", root.ID, root.ActiveSegmentID, agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}),
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
		RunID: "run_1", ItemID: itemID, Title: title, Rememberable: true,
		Tool: &agent.ToolCall{
			Kind: agent.ToolShell, Name: "shell", Command: "go test ./...", Status: agent.ToolRunning,
			ArgumentsJSON: []byte(`{"command":"go test ./...","timeoutMs":30000}`),
		},
	}
}
