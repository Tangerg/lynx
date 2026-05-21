package agui_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/lyra/internal/agui"
	"github.com/Tangerg/lynx/lyra/internal/service/chat"
)

// ts is a fixed timestamp every test uses so output ordering /
// JSON shape are deterministic. Translator copies it verbatim from
// the input event's BaseEvent.Timestamp.
var ts = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// base builds a chat.BaseEvent with the fixed test timestamp and
// the supplied (sessionID, turnID) pair.
func base(sid, tid string) chat.BaseEvent {
	return chat.BaseEvent{SessionID: sid, TurnID: tid, Timestamp: ts}
}

// TestTranslate_TurnStart maps to RunStarted carrying the right
// thread + run ids.
func TestTranslate_TurnStart(t *testing.T) {
	tr := agui.NewTranslator("s1", "r1")
	out := tr.Translate(chat.TurnStart{BaseEvent: base("s1", "r1"), Model: "claude"})

	if len(out) != 1 {
		t.Fatalf("want 1 event, got %d", len(out))
	}
	rs, ok := out[0].(agui.RunStarted)
	if !ok {
		t.Fatalf("want RunStarted, got %T", out[0])
	}
	if rs.ThreadID != "s1" || rs.RunID != "r1" {
		t.Errorf("ids = (%q,%q)", rs.ThreadID, rs.RunID)
	}
}

// TestTranslate_MessageDelta_LazyStart fires Start on first delta
// then bare Content events thereafter — sharing the same
// messageId for proper AG-UI message correlation.
func TestTranslate_MessageDelta_LazyStart(t *testing.T) {
	tr := agui.NewTranslator("s", "r")

	first := tr.Translate(chat.MessageDelta{BaseEvent: base("s", "r"), Text: "Hello "})
	if len(first) != 2 {
		t.Fatalf("first delta should produce Start+Content, got %d events", len(first))
	}
	start, ok := first[0].(agui.TextMessageStart)
	if !ok || start.Role != "assistant" {
		t.Fatalf("first[0] = %T (%v)", first[0], first[0])
	}
	c1, ok := first[1].(agui.TextMessageContent)
	if !ok || c1.Delta != "Hello " {
		t.Fatalf("first[1] = %#v", first[1])
	}
	if start.MessageID != c1.MessageID {
		t.Errorf("Start and Content must share messageId")
	}

	second := tr.Translate(chat.MessageDelta{BaseEvent: base("s", "r"), Text: "world"})
	if len(second) != 1 {
		t.Fatalf("subsequent delta should be Content only, got %d", len(second))
	}
	c2 := second[0].(agui.TextMessageContent)
	if c2.MessageID != c1.MessageID {
		t.Errorf("subsequent Content should reuse messageId; got %q vs %q", c2.MessageID, c1.MessageID)
	}
}

// TestTranslate_ToolCallStart_ClosesOpenText verifies that a tool
// call interrupting the assistant's text emits TextMessageEnd
// before the tool-call triplet — AG-UI requires the message to be
// closed before another stream begins.
func TestTranslate_ToolCallStart_ClosesOpenText(t *testing.T) {
	tr := agui.NewTranslator("s", "r")
	_ = tr.Translate(chat.MessageDelta{BaseEvent: base("s", "r"), Text: "thinking..."})

	out := tr.Translate(chat.ToolCallStart{
		BaseEvent: base("s", "r"),
		CallID:    "c-1",
		ToolName:  "bash",
		Arguments: `{"command":"ls"}`,
	})

	if len(out) != 4 {
		t.Fatalf("want TextMessageEnd + Start/Args/End, got %d events", len(out))
	}
	if _, ok := out[0].(agui.TextMessageEnd); !ok {
		t.Errorf("out[0] = %T, want TextMessageEnd", out[0])
	}
	ts, ok := out[1].(agui.ToolCallStart)
	if !ok || ts.ToolCallID != "c-1" || ts.ToolCallName != "bash" {
		t.Errorf("out[1] = %#v", out[1])
	}
	args, ok := out[2].(agui.ToolCallArgs)
	if !ok || args.Delta != `{"command":"ls"}` || args.ToolCallID != "c-1" {
		t.Errorf("out[2] = %#v", out[2])
	}
	if _, ok := out[3].(agui.ToolCallEnd); !ok {
		t.Errorf("out[3] = %T", out[3])
	}
}

// TestTranslate_ToolCallEnd_BecomesResult — Lyra collapses
// "tool finished + output" into one event, AG-UI separates them
// but the data arrives together.
func TestTranslate_ToolCallEnd_BecomesResult(t *testing.T) {
	tr := agui.NewTranslator("s", "r")

	out := tr.Translate(chat.ToolCallEnd{
		BaseEvent: base("s", "r"),
		CallID:    "c-1",
		Output:    "total 0\n",
	})
	if len(out) != 1 {
		t.Fatalf("want 1 event, got %d", len(out))
	}
	res, ok := out[0].(agui.ToolCallResult)
	if !ok || res.ToolCallID != "c-1" || res.Content != "total 0\n" {
		t.Errorf("out[0] = %#v", out[0])
	}
	if res.MessageID != "r" {
		t.Errorf("MessageID should be runID, got %q", res.MessageID)
	}
}

// TestTranslate_ToolCallEnd_ErrPrefersErrMessage — when the tool
// failed, the error message becomes the result content (a Lyra
// convention; AG-UI clients render Content verbatim).
func TestTranslate_ToolCallEnd_ErrPrefersErrMessage(t *testing.T) {
	tr := agui.NewTranslator("s", "r")
	out := tr.Translate(chat.ToolCallEnd{
		BaseEvent: base("s", "r"),
		CallID:    "c-1",
		Output:    "ignored",
		Err:       "command not found",
	})
	res := out[0].(agui.ToolCallResult)
	if res.Content != "command not found" {
		t.Errorf("err path should surface Err; got %q", res.Content)
	}
}

// TestTranslate_PlanGenerated_Custom emits a Custom event with
// name="plan_generated" since AG-UI v1 has no first-class plan
// event.
func TestTranslate_PlanGenerated_Custom(t *testing.T) {
	tr := agui.NewTranslator("s", "r")
	out := tr.Translate(chat.PlanGenerated{BaseEvent: base("s", "r"), Plan: "1. step"})
	if len(out) != 1 {
		t.Fatalf("want 1 event, got %d", len(out))
	}
	c, ok := out[0].(agui.Custom)
	if !ok {
		t.Fatalf("want Custom, got %T", out[0])
	}
	if c.Name != "plan_generated" {
		t.Errorf("name = %q", c.Name)
	}
	v := c.Value.(map[string]any)
	if v["plan"] != "1. step" || v["runId"] != "r" {
		t.Errorf("value = %#v", v)
	}
}

// TestTranslate_TurnEnd_ClosesOpenText is the canonical "happy
// path" close: streamed text → TextMessageEnd then RunFinished.
func TestTranslate_TurnEnd_ClosesOpenText(t *testing.T) {
	tr := agui.NewTranslator("s", "r")
	_ = tr.Translate(chat.MessageDelta{BaseEvent: base("s", "r"), Text: "done"})

	out := tr.Translate(chat.TurnEnd{BaseEvent: base("s", "r"), Reason: chat.TurnEndCompleted})
	if len(out) != 2 {
		t.Fatalf("want TextMessageEnd + RunFinished, got %d", len(out))
	}
	if _, ok := out[0].(agui.TextMessageEnd); !ok {
		t.Errorf("out[0] = %T", out[0])
	}
	if _, ok := out[1].(agui.RunFinished); !ok {
		t.Errorf("out[1] = %T", out[1])
	}
}

// TestTranslate_TurnEnd_Errored produces RunError instead of
// RunFinished on TurnEndErrored — clients see exactly one
// terminal event regardless of which path got here.
func TestTranslate_TurnEnd_Errored(t *testing.T) {
	tr := agui.NewTranslator("s", "r")
	out := tr.Translate(chat.TurnEnd{BaseEvent: base("s", "r"), Reason: chat.TurnEndErrored})
	if len(out) != 1 {
		t.Fatalf("want 1 event, got %d", len(out))
	}
	re, ok := out[0].(agui.RunError)
	if !ok {
		t.Fatalf("want RunError, got %T", out[0])
	}
	if re.Code != "TURN_ERRORED" {
		t.Errorf("code = %q", re.Code)
	}
}

// TestTranslate_ErrorEvent passes through the message + code
// verbatim — same shape the chat layer uses.
func TestTranslate_ErrorEvent(t *testing.T) {
	tr := agui.NewTranslator("s", "r")
	out := tr.Translate(chat.ErrorEvent{
		BaseEvent: base("s", "r"),
		Message:   "engine boom",
		Code:      "ENGINE_ERROR",
	})
	re := out[0].(agui.RunError)
	if re.Message != "engine boom" || re.Code != "ENGINE_ERROR" {
		t.Errorf("%#v", re)
	}
}

// TestSSEEncoder_FrameShape verifies the on-wire bytes match the
// SSE convention: `event: <type>\ndata: <json>\n\n`.
func TestSSEEncoder_FrameShape(t *testing.T) {
	buf := &bytes.Buffer{}
	enc := agui.NewSSEEncoder(buf)
	_ = enc.Encode(agui.RunFinished{Type: "RunFinished"})

	got := buf.String()
	if !strings.HasPrefix(got, "event: RunFinished\n") {
		t.Errorf("missing event line: %q", got)
	}
	if !strings.Contains(got, "data: ") || !strings.HasSuffix(got, "\n\n") {
		t.Errorf("frame shape wrong: %q", got)
	}
	// data line is parseable JSON
	dataLine := strings.SplitN(strings.TrimPrefix(got, "event: RunFinished\n"), "\n", 2)[0]
	dataLine = strings.TrimPrefix(dataLine, "data: ")
	var probe map[string]any
	if err := json.Unmarshal([]byte(dataLine), &probe); err != nil {
		t.Errorf("data not JSON: %v (%q)", err, dataLine)
	}
}
