package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/client"
)

// script is one of each event, small enough that the expected text can be read
// in full.
func script() []client.Envelope {
	events := []client.Event{
		client.RunStarted{RunID: "run_1", SessionID: "ses_1"},
		client.BlockCompleted{Block: client.Block{ID: "p", Kind: client.BlockUser, Text: "why?"}},
		client.PlanChanged{Items: []client.PlanItem{
			{Title: "Look", Status: client.PlanDone},
			{Title: "Fix", Status: client.PlanActive},
			{Title: "Verify", Status: client.PlanPending},
		}},
		client.BlockStarted{Block: client.Block{ID: "m", Kind: client.BlockAssistant}},
		client.BlockDelta{BlockID: "m", Text: "one "},
		client.BlockDelta{BlockID: "m", Text: "two"},
		client.BlockCompleted{Block: client.Block{ID: "m", Kind: client.BlockAssistant, Text: "one two"}},
		client.BlockStarted{Block: client.Block{ID: "t", Kind: client.BlockTool, Tool: &client.ToolCall{
			Kind: client.ToolShell, Name: "shell", Summary: "go test ./...", Status: client.ToolRunning,
		}}},
		client.BlockCompleted{Block: client.Block{ID: "t", Kind: client.BlockTool, Tool: &client.ToolCall{
			Kind: client.ToolShell, Name: "shell", Summary: "go test ./...", Status: client.ToolOK,
			Output: "ok\nFAIL", Duration: 1500 * time.Millisecond,
		}}},
		client.RunFinished{
			Outcome: client.Outcome{Status: client.OutcomeCompleted},
			Usage:   client.Usage{InputTokens: 12345, OutputTokens: 678, CachedTokens: 900, CostUSD: 0.0412, Duration: 2 * time.Second},
		},
	}
	out := make([]client.Envelope, len(events))
	for i, event := range events {
		out[i] = client.Envelope{ID: fmt.Sprintf("event_%d", i+1), Cursor: client.Cursor(i + 1), RunID: "run_1", SessionID: "ses_1", Event: event}
	}
	return out
}

func envelope(event client.Event) client.Envelope {
	return client.Envelope{ID: "event", Cursor: 1, Event: event}
}

func renderAll(t *testing.T, r interface {
	Render(client.Envelope) error
	Close() error
}, events []client.Envelope,
) {
	t.Helper()
	for _, ev := range events {
		if err := r.Render(ev); err != nil {
			t.Fatalf("Render(%T): %v", ev.Event, err)
		}
	}
	if err := r.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestTextOutput(t *testing.T) {
	var buf bytes.Buffer
	renderAll(t, NewText(&buf), script())

	want := strings.Join([]string{
		"",
		"› why?",
		"",
		"plan",
		"  ☑ Look",
		"  ▸ Fix",
		"  ☐ Verify",
		"",
		"one two",
		"",
		"● shell · go test ./...",
		"  │ ok",
		"  │ FAIL",
		"  ✓ 1.5s",
		"",
		"↑ 12,345  ↓ 678  cached 900  $0.0412  2.0s",
		"",
	}, "\n")
	if got := buf.String(); got != want {
		t.Fatalf("text output mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestTextStreamsAssistantProseAsItArrives(t *testing.T) {
	var buf bytes.Buffer
	text := NewText(&buf)

	if err := text.Render(envelope(client.BlockStarted{Block: client.Block{ID: "m", Kind: client.BlockAssistant}})); err != nil {
		t.Fatal(err)
	}
	if err := text.Render(envelope(client.BlockDelta{BlockID: "m", Text: "half"})); err != nil {
		t.Fatal(err)
	}
	// The point of streaming: the words are out before the block completes.
	if !strings.Contains(buf.String(), "half") {
		t.Fatalf("delta was buffered instead of written: %q", buf.String())
	}
}

func TestTextHoldsNonProseUntilItsBlockCompletes(t *testing.T) {
	var buf bytes.Buffer
	text := NewText(&buf)

	if err := text.Render(envelope(client.BlockStarted{Block: client.Block{ID: "r", Kind: client.BlockReasoning}})); err != nil {
		t.Fatal(err)
	}
	if err := text.Render(envelope(client.BlockDelta{BlockID: "r", Text: "thinking"})); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Fatalf("reasoning leaked before completing: %q", buf.String())
	}
	if err := text.Render(envelope(client.BlockCompleted{Block: client.Block{ID: "r", Kind: client.BlockReasoning}})); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); !strings.Contains(got, "· thinking") {
		t.Fatalf("completed reasoning = %q, want it prefixed with a marker", got)
	}
}

func TestTextIndentsContinuationLines(t *testing.T) {
	var buf bytes.Buffer
	text := NewText(&buf)
	if err := text.Render(envelope(client.BlockCompleted{Block: client.Block{
		ID: "r", Kind: client.BlockReasoning, Text: "first\nsecond",
	}})); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); !strings.Contains(got, "· first\n  second") {
		t.Fatalf("output = %q, want the second line aligned under the first", got)
	}
}

func TestTextCapsToolOutput(t *testing.T) {
	var lines []string
	for i := range maxToolOutputLines + 5 {
		lines = append(lines, "line"+string(rune('a'+i)))
	}
	var buf bytes.Buffer
	text := NewText(&buf)
	if err := text.Render(envelope(client.BlockCompleted{Block: client.Block{ID: "t", Kind: client.BlockTool, Tool: &client.ToolCall{
		Kind: client.ToolShell, Name: "shell", Status: client.ToolOK, Output: strings.Join(lines, "\n"),
	}}})); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if strings.Count(got, "  │ ") != maxToolOutputLines+1 {
		t.Fatalf("output body has %d lines, want %d plus one summary line", strings.Count(got, "  │ "), maxToolOutputLines)
	}
	if !strings.Contains(got, "… 5 more lines") {
		t.Fatalf("output = %q, want it to say how much was withheld", got)
	}
}

func TestTextReportsANonCompletedOutcome(t *testing.T) {
	var buf bytes.Buffer
	text := NewText(&buf)
	if err := text.Render(envelope(client.RunFinished{Outcome: client.Outcome{
		Status: client.OutcomeFailed, Error: "provider refused",
	}})); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); !strings.Contains(got, "failed: provider refused") {
		t.Fatalf("output = %q, want the failure stated", got)
	}
}

func TestJSONEmitsOneObjectPerEvent(t *testing.T) {
	var buf bytes.Buffer
	events := script()
	renderAll(t, NewJSON(&buf), events)

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != len(events) {
		t.Fatalf("wrote %d lines for %d events", len(lines), len(events))
	}
	want := []string{
		"run.started", "block.completed", "plan.changed",
		"block.started", "block.delta", "block.delta", "block.completed",
		"block.started", "block.completed", "run.finished",
	}
	for i, line := range lines {
		var got frame
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			t.Fatalf("line %d is not JSON: %v\n%s", i, err, line)
		}
		if got.Type != want[i] {
			t.Fatalf("line %d type = %q, want %q", i, got.Type, want[i])
		}
	}
}

func TestJSONCarriesTheToolProjection(t *testing.T) {
	var buf bytes.Buffer
	j := NewJSON(&buf)
	if err := j.Render(envelope(client.BlockCompleted{Block: client.Block{ID: "t", Kind: client.BlockTool, Tool: &client.ToolCall{
		Kind: client.ToolEdit, Name: "provider.patch", Path: "a.go", Summary: "change a.go",
		Status: client.ToolOK, Diff: "--- a\n+++ b", Duration: 250 * time.Millisecond,
	}}})); err != nil {
		t.Fatal(err)
	}
	var got frame
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if got.Block == nil || got.Block.Tool == nil {
		t.Fatalf("frame = %+v, want a tool projection", got)
	}
	tool := got.Block.Tool
	if tool.Kind != "edit" || tool.Name != "provider.patch" || tool.Path != "a.go" || tool.Status != "ok" || tool.Diff == "" || tool.DurationMS != 250 {
		t.Fatalf("tool = %+v, want the call carried through intact", tool)
	}
}

func TestJSONCarriesControlOptionsAndCompleteInteractionMetadata(t *testing.T) {
	events := []client.Envelope{
		envelope(client.RunStarted{
			RunID: "run_1", SessionID: "session_1",
			Options: client.RunOptions{Model: "deep", Mode: client.ModeReview, Permission: client.PermissionReadOnly, Effort: "high"},
		}),
		envelope(client.RunResumed{InterruptID: "approval_1"}),
		envelope(client.RunInterrupted{Interaction: client.Approval{
			InterruptID: "approval_2", Title: "Allow edit", Detail: "change file",
			Risk: "writes", Diff: "--- a\n+++ b", RuleHint: "edit:a.go",
		}}),
		envelope(client.RunInterrupted{Interaction: client.Question{
			InterruptID: "question_1", Title: "Choose", Detail: "select strategy",
			Fields: []client.QuestionField{{
				ID: "strategy", Label: "Strategy", Description: "How to proceed",
				Kind: client.QuestionSingle, Required: true, Placeholder: "pick one",
				Options: []client.QuestionOption{{
					Value: "safe", Label: "Safe", Description: "Prefer safety", Recommended: true,
				}},
			}},
		}}),
	}
	var output bytes.Buffer
	renderAll(t, NewJSON(&output), events)
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	var started, resumed, approval, question frame
	decodeFrames(t, lines, &started, &resumed, &approval, &question)
	requireRunOptions(t, started.Options)
	if resumed.InterruptID != "approval_1" {
		t.Fatalf("resume frame = %+v", resumed)
	}
	if approval.Interaction == nil {
		t.Fatalf("approval frame = %+v", approval.Interaction)
	}
	if approval.Interaction.RuleHint != "edit:a.go" {
		t.Fatalf("approval frame = %+v", approval.Interaction)
	}
	if question.Interaction == nil {
		t.Fatalf("question frame = %+v", question.Interaction)
	}
	requireQuestionField(t, question.Interaction.Fields)
}

func decodeFrames(t *testing.T, lines []string, targets ...*frame) {
	t.Helper()
	if len(lines) != len(targets) {
		t.Fatalf("JSON lines = %d, want %d", len(lines), len(targets))
	}
	for index, target := range targets {
		if err := json.Unmarshal([]byte(lines[index]), target); err != nil {
			t.Fatal(err)
		}
	}
}

func requireRunOptions(t *testing.T, options *runOptionsJSON) {
	t.Helper()
	if options == nil {
		t.Fatal("run options are absent")
	}
	want := runOptionsJSON{Model: "deep", Mode: "review", Permission: "read-only", Effort: "high"}
	if *options != want {
		t.Fatalf("run options = %+v, want %+v", options, want)
	}
}

func requireQuestionField(t *testing.T, fields []questionFieldJSON) {
	t.Helper()
	if len(fields) != 1 {
		t.Fatalf("question fields = %+v", fields)
	}
	field := fields[0]
	if field.Description != "How to proceed" || field.Placeholder != "pick one" {
		t.Fatalf("question field = %+v", field)
	}
	if len(field.Options) != 1 {
		t.Fatalf("question options = %+v", field.Options)
	}
	option := field.Options[0]
	if !option.Recommended || option.Description != "Prefer safety" {
		t.Fatalf("question option = %+v", option)
	}
}

func TestRenderersRejectInvalidEventsInsteadOfEmittingEmptyOutput(t *testing.T) {
	for name, renderer := range map[string]interface {
		Render(client.Envelope) error
		Close() error
	}{
		"json": NewJSON(&bytes.Buffer{}),
		"text": NewText(&bytes.Buffer{}),
	} {
		t.Run(name, func(t *testing.T) {
			if err := renderer.Render(envelope(nil)); err == nil || !strings.Contains(err.Error(), "event is nil") {
				t.Fatalf("Render error = %v", err)
			}
			if err := renderer.Render(envelope(client.RunResumed{InterruptID: "later"})); err == nil || !strings.Contains(err.Error(), "event is nil") {
				t.Fatalf("sticky Render error = %v", err)
			}
		})
	}
}

func TestRenderersCarryUserAttachments(t *testing.T) {
	block := client.Block{
		ID: "u", Kind: client.BlockUser, Text: "inspect",
		Attachments: []client.Attachment{{
			ID: "att_1", Kind: client.AttachmentText, Name: "notes.txt",
			Path: "/tmp/notes.txt", MimeType: "text/plain", Size: 5,
		}},
	}
	var textOut bytes.Buffer
	if err := NewText(&textOut).Render(envelope(client.BlockCompleted{Block: block})); err != nil {
		t.Fatal(err)
	}
	if got := textOut.String(); !strings.Contains(got, "@ notes.txt (text/plain, 5 bytes)") {
		t.Fatalf("text attachment = %q", got)
	}

	var jsonOut bytes.Buffer
	if err := NewJSON(&jsonOut).Render(envelope(client.BlockCompleted{Block: block})); err != nil {
		t.Fatal(err)
	}
	var got frame
	if err := json.Unmarshal(jsonOut.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Block == nil || len(got.Block.Attachments) != 1 || got.Block.Attachments[0].Path != "/tmp/notes.txt" {
		t.Fatalf("json attachment = %+v", got.Block)
	}
}

func TestJSONOmitsWhatDidNotHappen(t *testing.T) {
	var buf bytes.Buffer
	j := NewJSON(&buf)
	if err := j.Render(envelope(client.BlockDelta{BlockID: "m", Text: "x"})); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	for _, absent := range []string{"block", "usage", "outcome", "approval", "plan", "runId"} {
		if _, ok := got[absent]; ok {
			t.Fatalf("delta frame carries %q: %v", absent, got)
		}
	}
	if got["blockId"] != "m" || got["text"] != "x" {
		t.Fatalf("delta frame = %v, want the block id and the text", got)
	}
}

func TestThousands(t *testing.T) {
	for _, tc := range []struct {
		in   int64
		want string
	}{
		{0, "0"}, {7, "7"}, {999, "999"}, {1000, "1,000"},
		{12345, "12,345"}, {1000000, "1,000,000"}, {-4321, "-4,321"},
		{-1 << 63, "-9,223,372,036,854,775,808"},
	} {
		if got := thousands(tc.in); got != tc.want {
			t.Errorf("thousands(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDuration(t *testing.T) {
	for _, tc := range []struct {
		in   time.Duration
		want string
	}{
		{120 * time.Millisecond, "120ms"},
		{999 * time.Millisecond, "999ms"},
		{time.Second, "1.0s"},
		{2500 * time.Millisecond, "2.5s"},
	} {
		if got := duration(tc.in); got != tc.want {
			t.Errorf("duration(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
