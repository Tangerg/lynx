package agui_test

import (
	"strings"
	"testing"
	"time"

	aguievents "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"

	"github.com/Tangerg/lynx/lyra/internal/agui"
	"github.com/Tangerg/lynx/lyra/internal/service/approval"
	"github.com/Tangerg/lynx/lyra/internal/service/chat"
)

// approvalRequest is the test-shape minimum needed to feed a
// ToolCallApproval into the translator — saves repeating the
// approval.Request literal in every test.
func approvalRequest(id, toolName string) approval.Request {
	return approval.Request{ID: id, ToolName: toolName, RequestedAt: ts}
}

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
// before the tool-call triplet, with a StepStarted bracketing the
// tool lifecycle.
func TestTranslate_ToolCallStart_ClosesOpenText(t *testing.T) {
	tr := agui.NewTranslator("s", "r")
	_ = tr.Translate(chat.MessageDelta{BaseEvent: base("s", "r"), Text: "thinking..."})

	out := tr.Translate(chat.ToolCallStart{
		BaseEvent: base("s", "r"),
		CallID:    "c-1",
		ToolName:  "bash",
		Arguments: `{"command":"ls"}`,
	})

	// TextMessageEnd + StepStarted + ToolCallStart + Args + End
	if len(out) != 5 {
		t.Fatalf("want 5 events (TextEnd + StepStart + tool triplet), got %d", len(out))
	}
	if _, ok := out[0].(*aguievents.TextMessageEndEvent); !ok {
		t.Errorf("out[0] = %T, want *TextMessageEndEvent", out[0])
	}
	if step, ok := out[1].(*aguievents.StepStartedEvent); !ok || step.StepName != "tool:bash" {
		t.Errorf("out[1] = %#v, want StepStarted(tool:bash)", out[1])
	}
	tcs, ok := out[2].(*aguievents.ToolCallStartEvent)
	if !ok || tcs.ToolCallID != "c-1" || tcs.ToolCallName != "bash" {
		t.Errorf("out[2] = %#v", out[2])
	}
	args, ok := out[3].(*aguievents.ToolCallArgsEvent)
	if !ok || args.Delta != `{"command":"ls"}` || args.ToolCallID != "c-1" {
		t.Errorf("out[3] = %#v", out[3])
	}
	if _, ok := out[4].(*aguievents.ToolCallEndEvent); !ok {
		t.Errorf("out[4] = %T", out[4])
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

// TestTranslate_PlanGenerated_Custom emits the Step-bracketed
// "plan_generated" custom event so plan review surfaces as a
// named phase on the wire.
func TestTranslate_PlanGenerated_Custom(t *testing.T) {
	tr := agui.NewTranslator("s", "r")
	out := tr.Translate(chat.PlanGenerated{BaseEvent: base("s", "r"), Plan: "1. step"})
	if len(out) != 3 {
		t.Fatalf("want StepStart + Custom + StepFinish, got %d events", len(out))
	}
	if step, ok := out[0].(*aguievents.StepStartedEvent); !ok || step.StepName != "plan_review" {
		t.Errorf("out[0] = %#v, want StepStarted(plan_review)", out[0])
	}
	c, ok := out[1].(*aguievents.CustomEvent)
	if !ok {
		t.Fatalf("out[1] = %T, want *CustomEvent", out[1])
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
	if step, ok := out[2].(*aguievents.StepFinishedEvent); !ok || step.StepName != "plan_review" {
		t.Errorf("out[2] = %#v, want StepFinished(plan_review)", out[2])
	}
}

// TestTranslate_CompactBoundary_Custom — a compaction boundary
// surfaces as a standalone "compact_boundary" CustomEvent carrying
// the before/after message counts (no Step bracket — it's a
// notification, not an interactive phase).
func TestTranslate_CompactBoundary_Custom(t *testing.T) {
	tr := agui.NewTranslator("s", "r")
	out := tr.Translate(chat.CompactBoundary{BaseEvent: base("s", "r"), MessagesBefore: 120, MessagesAfter: 40})
	if len(out) != 1 {
		t.Fatalf("want a single CustomEvent, got %d events", len(out))
	}
	c, ok := out[0].(*aguievents.CustomEvent)
	if !ok || c.Name != "compact_boundary" {
		t.Fatalf("out[0] = %#v, want CustomEvent(compact_boundary)", out[0])
	}
	v, ok := c.Value.(map[string]any)
	if !ok || v["messagesBefore"] != 120 || v["messagesAfter"] != 40 {
		t.Errorf("value = %#v", c.Value)
	}
}

// TestTranslate_MemoryUpdated_Custom — a memory write surfaces as a
// standalone "memory_updated" CustomEvent carrying the saved facts.
func TestTranslate_MemoryUpdated_Custom(t *testing.T) {
	tr := agui.NewTranslator("s", "r")
	out := tr.Translate(chat.MemoryUpdated{BaseEvent: base("s", "r"), Facts: "- prefers tabs"})
	if len(out) != 1 {
		t.Fatalf("want a single CustomEvent, got %d events", len(out))
	}
	c, ok := out[0].(*aguievents.CustomEvent)
	if !ok || c.Name != "memory_updated" {
		t.Fatalf("out[0] = %#v, want CustomEvent(memory_updated)", out[0])
	}
	if v, ok := c.Value.(map[string]any); !ok || v["facts"] != "- prefers tabs" {
		t.Errorf("value = %#v", c.Value)
	}
}

// TestTranslate_MaintenanceCustom_ClosesOpenText — a maintenance
// CustomEvent emitted while an assistant text stream is still open
// first closes that stream, keeping the wire balanced.
func TestTranslate_MaintenanceCustom_ClosesOpenText(t *testing.T) {
	tr := agui.NewTranslator("s", "r")
	_ = tr.Translate(chat.MessageDelta{BaseEvent: base("s", "r"), Text: "hi"}) // opens text
	out := tr.Translate(chat.CompactBoundary{BaseEvent: base("s", "r"), MessagesBefore: 10, MessagesAfter: 3})
	if len(out) != 2 {
		t.Fatalf("want TextMessageEnd + CustomEvent, got %d events", len(out))
	}
	if _, ok := out[0].(*aguievents.TextMessageEndEvent); !ok {
		t.Errorf("out[0] = %T, want TextMessageEnd (open text closed first)", out[0])
	}
	if c, ok := out[1].(*aguievents.CustomEvent); !ok || c.Name != "compact_boundary" {
		t.Errorf("out[1] = %#v, want CustomEvent(compact_boundary)", out[1])
	}
}

// TestTranslate_ToolStepLifecycle — tool calls emit a balanced
// StepStarted("tool:<name>") + StepFinished pair around the
// AG-UI tool triplet so clients can render the tool's progress
// as a discrete step.
func TestTranslate_ToolStepLifecycle(t *testing.T) {
	tr := agui.NewTranslator("s", "r")
	out := tr.Translate(chat.ToolCallStart{
		BaseEvent: base("s", "r"),
		CallID:    "c-1",
		ToolName:  "bash",
		Arguments: `{"command":"ls"}`,
	})
	out = append(out, tr.Translate(chat.ToolCallEnd{
		BaseEvent: base("s", "r"),
		CallID:    "c-1",
		Output:    "total 0\n",
	})...)

	// Expected order: StepStarted, ToolCallStart, Args, End,
	// ToolCallResult, StepFinished.
	if len(out) != 6 {
		t.Fatalf("want 6 events, got %d", len(out))
	}
	if step, ok := out[0].(*aguievents.StepStartedEvent); !ok || step.StepName != "tool:bash" {
		t.Errorf("out[0] = %#v, want StepStarted(tool:bash)", out[0])
	}
	if step, ok := out[5].(*aguievents.StepFinishedEvent); !ok || step.StepName != "tool:bash" {
		t.Errorf("out[5] = %#v, want StepFinished(tool:bash)", out[5])
	}
}

// TestTranslate_ApprovalStepClosesOnToolStart — the approval Step
// is opened by ToolCallApproval and closed when the corresponding
// ToolCallStart fires (model proceeds after approval).
func TestTranslate_ApprovalStepClosesOnToolStart(t *testing.T) {
	tr := agui.NewTranslator("s", "r")
	_ = tr.Translate(chat.ToolCallApproval{
		BaseEvent: base("s", "r"),
		Request:   approvalRequest("c-1", "bash"),
	})
	out := tr.Translate(chat.ToolCallStart{
		BaseEvent: base("s", "r"),
		CallID:    "c-1",
		ToolName:  "bash",
		Arguments: `{}`,
	})
	// Expected ordering: StepFinished(approval:bash) +
	// StepStarted(tool:bash) + tool triplet.
	if len(out) != 5 {
		t.Fatalf("want 5 events, got %d (%+v)", len(out), out)
	}
	if step, ok := out[0].(*aguievents.StepFinishedEvent); !ok || step.StepName != "approval:bash" {
		t.Errorf("out[0] = %#v, want StepFinished(approval:bash)", out[0])
	}
	if step, ok := out[1].(*aguievents.StepStartedEvent); !ok || step.StepName != "tool:bash" {
		t.Errorf("out[1] = %#v, want StepStarted(tool:bash)", out[1])
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
// serializes a translated event into the canonical AG-UI shape —
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
