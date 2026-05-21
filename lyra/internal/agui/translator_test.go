package agui_test

import (
	"strings"
	"testing"
	"time"

	aguievents "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"

	"github.com/Tangerg/lynx/lyra/internal/agui"
	"github.com/Tangerg/lynx/lyra/internal/service/chat"
)

// ts is a fixed timestamp every test uses so output ordering /
// JSON shape are deterministic. Translator passes the chat
// event's timestamp through; we don't assert on the AG-UI side
// because the SDK fills its own ms timestamp.
var ts = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// base builds a chat.BaseEvent with the fixed test timestamp and
// the supplied (sessionID, turnID) pair.
func base(sid, tid string) chat.BaseEvent {
	return chat.BaseEvent{SessionID: sid, TurnID: tid, Timestamp: ts}
}

// TestTranslate_TurnStart maps to a RunStartedEvent carrying the
// right thread + run ids.
func TestTranslate_TurnStart(t *testing.T) {
	tr := agui.NewTranslator("s1", "r1")
	out := tr.Translate(chat.TurnStart{BaseEvent: base("s1", "r1"), Model: "claude"})

	if len(out) != 1 {
		t.Fatalf("want 1 event, got %d", len(out))
	}
	rs, ok := out[0].(*aguievents.RunStartedEvent)
	if !ok {
		t.Fatalf("want *RunStartedEvent, got %T", out[0])
	}
	if rs.ThreadIDValue != "s1" || rs.RunIDValue != "r1" {
		t.Errorf("ids = (%q,%q)", rs.ThreadIDValue, rs.RunIDValue)
	}
}

// TestTranslate_MessageDelta_LazyStart fires Start on first delta
// then bare Content events thereafter — sharing the same
// messageId.
func TestTranslate_MessageDelta_LazyStart(t *testing.T) {
	tr := agui.NewTranslator("s", "r")

	first := tr.Translate(chat.MessageDelta{BaseEvent: base("s", "r"), Text: "Hello "})
	if len(first) != 2 {
		t.Fatalf("first delta should produce Start+Content, got %d events", len(first))
	}
	start, ok := first[0].(*aguievents.TextMessageStartEvent)
	if !ok {
		t.Fatalf("first[0] = %T", first[0])
	}
	if start.Role == nil || *start.Role != "assistant" {
		t.Errorf("Role = %v", start.Role)
	}
	c1, ok := first[1].(*aguievents.TextMessageContentEvent)
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
	c2 := second[0].(*aguievents.TextMessageContentEvent)
	if c2.MessageID != c1.MessageID {
		t.Errorf("subsequent Content should reuse messageId; got %q vs %q", c2.MessageID, c1.MessageID)
	}
}

// TestTranslate_ToolCallStart_ClosesOpenText verifies that a tool
// call interrupting the assistant's text emits TextMessageEnd
// before the tool-call triplet.
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
	if _, ok := out[0].(*aguievents.TextMessageEndEvent); !ok {
		t.Errorf("out[0] = %T, want *TextMessageEndEvent", out[0])
	}
	tcs, ok := out[1].(*aguievents.ToolCallStartEvent)
	if !ok || tcs.ToolCallID != "c-1" || tcs.ToolCallName != "bash" {
		t.Errorf("out[1] = %#v", out[1])
	}
	args, ok := out[2].(*aguievents.ToolCallArgsEvent)
	if !ok || args.Delta != `{"command":"ls"}` || args.ToolCallID != "c-1" {
		t.Errorf("out[2] = %#v", out[2])
	}
	if _, ok := out[3].(*aguievents.ToolCallEndEvent); !ok {
		t.Errorf("out[3] = %T", out[3])
	}
}

// TestTranslate_ToolCallEnd_BecomesResult — Lyra collapses
// "tool finished + output" into one event; AG-UI separates them
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
	res, ok := out[0].(*aguievents.ToolCallResultEvent)
	if !ok || res.ToolCallID != "c-1" || res.Content != "total 0\n" {
		t.Errorf("out[0] = %#v", out[0])
	}
	if res.MessageID != "r" {
		t.Errorf("MessageID should be runID, got %q", res.MessageID)
	}
}

// TestTranslate_ToolCallEnd_ErrPrefersErrMessage — when the tool
// failed, the error message becomes the result content.
func TestTranslate_ToolCallEnd_ErrPrefersErrMessage(t *testing.T) {
	tr := agui.NewTranslator("s", "r")
	out := tr.Translate(chat.ToolCallEnd{
		BaseEvent: base("s", "r"),
		CallID:    "c-1",
		Output:    "ignored",
		Err:       "command not found",
	})
	res := out[0].(*aguievents.ToolCallResultEvent)
	if res.Content != "command not found" {
		t.Errorf("err path should surface Err; got %q", res.Content)
	}
}

// TestTranslate_PlanGenerated_Custom emits a CustomEvent with
// name="plan_generated".
func TestTranslate_PlanGenerated_Custom(t *testing.T) {
	tr := agui.NewTranslator("s", "r")
	out := tr.Translate(chat.PlanGenerated{BaseEvent: base("s", "r"), Plan: "1. step"})
	if len(out) != 1 {
		t.Fatalf("want 1 event, got %d", len(out))
	}
	c, ok := out[0].(*aguievents.CustomEvent)
	if !ok {
		t.Fatalf("want *CustomEvent, got %T", out[0])
	}
	if c.Name != "plan_generated" {
		t.Errorf("name = %q", c.Name)
	}
	v, ok := c.Value.(map[string]any)
	if !ok {
		t.Fatalf("value not a map: %#v", c.Value)
	}
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
	if _, ok := out[0].(*aguievents.TextMessageEndEvent); !ok {
		t.Errorf("out[0] = %T", out[0])
	}
	if _, ok := out[1].(*aguievents.RunFinishedEvent); !ok {
		t.Errorf("out[1] = %T", out[1])
	}
}

// TestTranslate_TurnEnd_Errored produces RunError on
// TurnEndErrored.
func TestTranslate_TurnEnd_Errored(t *testing.T) {
	tr := agui.NewTranslator("s", "r")
	out := tr.Translate(chat.TurnEnd{BaseEvent: base("s", "r"), Reason: chat.TurnEndErrored})
	if len(out) != 1 {
		t.Fatalf("want 1 event, got %d", len(out))
	}
	re, ok := out[0].(*aguievents.RunErrorEvent)
	if !ok {
		t.Fatalf("want *RunErrorEvent, got %T", out[0])
	}
	if re.Code == nil || *re.Code != "TURN_ERRORED" {
		t.Errorf("code = %v", re.Code)
	}
}

// TestTranslate_ErrorEvent passes through the message + code
// verbatim.
func TestTranslate_ErrorEvent(t *testing.T) {
	tr := agui.NewTranslator("s", "r")
	out := tr.Translate(chat.ErrorEvent{
		BaseEvent: base("s", "r"),
		Message:   "engine boom",
		Code:      "ENGINE_ERROR",
	})
	re := out[0].(*aguievents.RunErrorEvent)
	if re.Message != "engine boom" {
		t.Errorf("message = %q", re.Message)
	}
	if re.Code == nil || *re.Code != "ENGINE_ERROR" {
		t.Errorf("code = %v", re.Code)
	}
}

// TestTranslate_EventToJSON_RunStarted verifies the SDK
// serialises a translated event into the canonical AG-UI shape —
// `type` discriminator + the documented field names.
func TestTranslate_EventToJSON_RunStarted(t *testing.T) {
	tr := agui.NewTranslator("s1", "r1")
	out := tr.Translate(chat.TurnStart{BaseEvent: base("s1", "r1")})

	rs := out[0].(*aguievents.RunStartedEvent)
	data, err := rs.ToJSON()
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, want := range []string{
		`"type":"RUN_STARTED"`,
		`"threadId":"s1"`,
		`"runId":"r1"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in %s", want, body)
		}
	}
}
