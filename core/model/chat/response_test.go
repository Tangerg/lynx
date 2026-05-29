package chat_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
)

func TestFinishReason_String(t *testing.T) {
	if chat.FinishReasonStop.String() != "stop" {
		t.Fatalf("String = %q, want stop", chat.FinishReasonStop.String())
	}
}

func TestNewResult_RejectsNilInputs(t *testing.T) {
	if _, err := chat.NewResult(nil, &chat.ResultMetadata{}); err == nil {
		t.Fatal("nil assistant message must error")
	}
	if _, err := chat.NewResult(chat.NewAssistantMessage(chat.MessageParams{}), nil); err == nil {
		t.Fatal("nil metadata must error")
	}
}

func TestNewResult_Builds(t *testing.T) {
	msg := chat.NewAssistantMessage(chat.MessageParams{Text: "hi"})
	meta := &chat.ResultMetadata{FinishReason: chat.FinishReasonStop}

	got, err := chat.NewResult(msg, meta)
	if err != nil {
		t.Fatal(err)
	}
	if got.AssistantMessage != msg {
		t.Fatal("AssistantMessage not threaded through")
	}
	if got.Metadata != meta {
		t.Fatal("Metadata not threaded through")
	}
	if got.ToolMessage != nil {
		t.Fatalf("expected nil ToolMessage, got %v", got.ToolMessage)
	}
}

func TestResultMetadata_GetSet(t *testing.T) {
	meta := &chat.ResultMetadata{}

	if v, ok := meta.Get("missing"); ok || v != nil {
		t.Fatalf("Get(missing) = (%v,%v), want (nil,false)", v, ok)
	}
	meta.Set("k", "v")
	if v, _ := meta.Get("k"); v != "v" {
		t.Fatalf("Get(k) = %v, want v", v)
	}
}

func TestResponseMetadata_GetSet(t *testing.T) {
	meta := &chat.ResponseMetadata{}

	if _, ok := meta.Get("absent"); ok {
		t.Fatal("absent key must report ok=false")
	}
	meta.Set("trace", "abc")
	if v, _ := meta.Get("trace"); v != "abc" {
		t.Fatalf("Get(trace) = %v, want abc", v)
	}
}

func TestNewResponse_Validates(t *testing.T) {
	meta := &chat.ResponseMetadata{}

	if _, err := chat.NewResponse(nil, meta); err == nil {
		t.Fatal("nil result must error")
	}

	res, _ := chat.NewResult(
		chat.NewAssistantMessage(chat.MessageParams{Text: "hi"}),
		&chat.ResultMetadata{},
	)
	if _, err := chat.NewResponse(res, nil); err == nil {
		t.Fatal("nil metadata must error")
	}

	resp, err := chat.NewResponse(res, meta)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Result != res {
		t.Fatal("Response.Result must reference the supplied result")
	}
}

func TestNewResponse_ErrorMessageMentionsCause(t *testing.T) {
	_, err := chat.NewResponse(nil, &chat.ResponseMetadata{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "result") {
		t.Fatalf("error %q should mention result", err.Error())
	}
}

// TestResponse_StreamAccessors covers the streaming-chunk accessors a
// tool-loop consumer reads per chunk: a text chunk yields TextDelta only,
// a reasoning-only chunk yields ReasoningDelta only, the middleware's
// tool-result chunk reports IsToolResult, and all three are nil-safe.
func TestResponse_StreamAccessors(t *testing.T) {
	textResp := &chat.Response{Result: &chat.Result{
		AssistantMessage: chat.NewAssistantMessage(chat.MessageParams{Text: "hello"}),
	}}
	if got := textResp.TextDelta(); got != "hello" {
		t.Fatalf("TextDelta = %q, want hello", got)
	}
	if textResp.ReasoningDelta() != "" {
		t.Fatal("a text chunk carries no reasoning")
	}
	if textResp.IsToolResult() {
		t.Fatal("a text chunk is not a tool result")
	}

	reasoningResp := &chat.Response{Result: &chat.Result{
		AssistantMessage: chat.NewAssistantMessage([]chat.OutputPart{&chat.ReasoningPart{Text: "thinking"}}),
	}}
	if got := reasoningResp.ReasoningDelta(); got != "thinking" {
		t.Fatalf("ReasoningDelta = %q, want thinking", got)
	}
	if reasoningResp.TextDelta() != "" {
		t.Fatal("a reasoning-only chunk carries no text")
	}

	// Tool-result chunk = round boundary: ToolMessage set, AssistantMessage nil.
	toolResp := &chat.Response{Result: &chat.Result{ToolMessage: &chat.ToolMessage{}}}
	if !toolResp.IsToolResult() {
		t.Fatal("a ToolMessage-only chunk should report IsToolResult")
	}
	if toolResp.TextDelta() != "" || toolResp.ReasoningDelta() != "" {
		t.Fatal("a tool-result chunk carries no assistant text/reasoning")
	}

	// nil receiver + empty response must be safe and falsy.
	var nilResp *chat.Response
	empty := &chat.Response{}
	for _, r := range []*chat.Response{nilResp, empty} {
		if r.TextDelta() != "" || r.ReasoningDelta() != "" || r.IsToolResult() {
			t.Fatalf("accessors must be safe + falsy on %#v", r)
		}
	}
}
