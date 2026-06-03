package openai_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/openai"
)

func newResponsesModel(t *testing.T, baseURL, modelID string) *openai.ResponsesChatModel {
	t.Helper()
	opts, err := chat.NewOptions(modelID)
	if err != nil {
		t.Fatalf("NewOptions: %v", err)
	}
	m, err := openai.NewResponsesChatModel(openai.ChatModelConfig{
		APIKey:         model.NewAPIKey("test-key"),
		DefaultOptions: opts,
		RequestOptions: []option.RequestOption{option.WithBaseURL(baseURL)},
	})
	if err != nil {
		t.Fatalf("NewResponsesChatModel: %v", err)
	}
	return m
}

// Single-shot /v1/responses payload: a reasoning item, then text, then
// a function_call, then more text — exactly the interleaved shape the
// Responses API gives us (and Chat Completions cannot).
const responsesInterleavedJSON = `{
  "id": "resp_abc",
  "object": "response",
  "model": "gpt-5",
  "created_at": 1700000000,
  "status": "completed",
  "error": null,
  "incomplete_details": null,
  "instructions": null,
  "metadata": null,
  "output": [
    {"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"想想看"}],"encrypted_content":"enc_xyz","status":"completed"},
    {"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"先查天气：","annotations":[]}]},
    {"type":"function_call","id":"fc_1","call_id":"call_w","name":"weather","arguments":"{\"city\":\"BJ\"}","status":"completed"},
    {"type":"message","id":"msg_2","role":"assistant","status":"completed","content":[{"type":"output_text","text":"等结果。","annotations":[]}]}
  ],
  "parallel_tool_calls": false,
  "temperature": 1,
  "tool_choice": "auto",
  "tools": [],
  "top_p": 1,
  "usage": {
    "input_tokens": 12,
    "output_tokens": 8,
    "total_tokens": 20,
    "input_tokens_details": {"cached_tokens": 0},
    "output_tokens_details": {"reasoning_tokens": 3}
  }
}`

func TestResponsesChatModel_Call_InterleavedOutput(t *testing.T) {
	var seenURL string
	srv := testutil.JSONServer(http.StatusOK, responsesInterleavedJSON, func(r *http.Request) {
		seenURL = r.URL.Path
	})
	t.Cleanup(srv.Close)

	m := newResponsesModel(t, srv.URL, "gpt-5")
	req, _ := chat.NewRequest([]chat.Message{chat.NewUserMessage("查天气")})

	resp, err := m.Call(t.Context(), req)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !strings.Contains(seenURL, "responses") {
		t.Errorf("URL = %q; want /v1/responses", seenURL)
	}

	msg := resp.Result.AssistantMessage
	if msg == nil {
		t.Fatal("AssistantMessage is nil")
	}
	if len(msg.Parts) != 4 {
		t.Fatalf("Parts len = %d; want 4", len(msg.Parts))
	}
	wantKinds := []chat.PartKind{
		chat.PartKindReasoning,
		chat.PartKindText,
		chat.PartKindToolCall,
		chat.PartKindText,
	}
	for i, p := range msg.Parts {
		if p.Kind() != wantKinds[i] {
			t.Errorf("Parts[%d].Kind() = %s; want %s", i, p.Kind(), wantKinds[i])
		}
	}

	reasoning := msg.Parts[0].(*chat.ReasoningPart)
	if reasoning.Text != "想想看" {
		t.Errorf("reasoning text = %q", reasoning.Text)
	}
	if string(reasoning.Signature) != "enc_xyz" {
		t.Errorf("reasoning signature = %q; want enc_xyz", reasoning.Signature)
	}

	if msg.Parts[1].(*chat.TextPart).Text != "先查天气：" {
		t.Errorf("text[0] = %q", msg.Parts[1].(*chat.TextPart).Text)
	}
	if msg.Parts[3].(*chat.TextPart).Text != "等结果。" {
		t.Errorf("text[1] = %q", msg.Parts[3].(*chat.TextPart).Text)
	}

	tc := msg.Parts[2].(*chat.ToolCallPart)
	if tc.ID != "call_w" || tc.Name != "weather" || tc.Arguments != `{"city":"BJ"}` {
		t.Errorf("tool call = %+v", tc)
	}

	if resp.Result.Metadata.FinishReason != chat.FinishReasonToolCalls {
		t.Errorf("FinishReason = %q; want tool_calls", resp.Result.Metadata.FinishReason)
	}
	if resp.Metadata.Usage == nil {
		t.Fatal("usage missing")
	}
	if resp.Metadata.Usage.PromptTokens != 12 || resp.Metadata.Usage.CompletionTokens != 8 {
		t.Errorf("usage tokens = %+v", resp.Metadata.Usage)
	}
	if resp.Metadata.Usage.ReasoningTokens == nil || *resp.Metadata.Usage.ReasoningTokens != 3 {
		t.Errorf("reasoning tokens not surfaced: %+v", resp.Metadata.Usage.ReasoningTokens)
	}
}

func TestResponsesChatModel_Stream_InterleavedDeltas(t *testing.T) {
	// Build the SSE event sequence by hand. Each event ships exactly one
	// part delta to lynx — reasoning → text → tool_call → text — and the
	// final response.completed carries usage + finish reason.
	events := []testutil.AnthropicEvent{
		{Event: "response.created", Data: `{"type":"response.created","sequence_number":1,"response":{"id":"resp_x","object":"response","model":"gpt-5","created_at":1700000000,"status":"in_progress","error":null,"incomplete_details":null,"instructions":null,"metadata":null,"output":[],"parallel_tool_calls":false,"temperature":1,"tool_choice":"auto","tools":[],"top_p":1}}`},

		// reasoning item: added (id pickup) + text delta + done (signature)
		{Event: "response.output_item.added", Data: `{"type":"response.output_item.added","sequence_number":2,"output_index":0,"item":{"type":"reasoning","id":"rs_1","summary":[],"status":"in_progress"}}`},
		{Event: "response.reasoning_text.delta", Data: `{"type":"response.reasoning_text.delta","sequence_number":3,"item_id":"rs_1","output_index":0,"content_index":0,"delta":"想想看"}`},
		{Event: "response.output_item.done", Data: `{"type":"response.output_item.done","sequence_number":4,"output_index":0,"item":{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"想想看"}],"encrypted_content":"enc_xyz","status":"completed"}}`},

		// first text message: added + delta
		{Event: "response.output_item.added", Data: `{"type":"response.output_item.added","sequence_number":5,"output_index":1,"item":{"type":"message","id":"msg_1","role":"assistant","status":"in_progress","content":[]}}`},
		{Event: "response.output_text.delta", Data: `{"type":"response.output_text.delta","sequence_number":6,"item_id":"msg_1","output_index":1,"content_index":0,"delta":"先查天气：","logprobs":[]}`},

		// function call: added (gets id mapping rs_1 → call_w) + arg delta
		{Event: "response.output_item.added", Data: `{"type":"response.output_item.added","sequence_number":7,"output_index":2,"item":{"type":"function_call","id":"fc_1","call_id":"call_w","name":"weather","arguments":"","status":"in_progress"}}`},
		{Event: "response.function_call_arguments.delta", Data: `{"type":"response.function_call_arguments.delta","sequence_number":8,"item_id":"fc_1","output_index":2,"delta":"{\"city\":\"BJ\"}"}`},

		// trailing text
		{Event: "response.output_item.added", Data: `{"type":"response.output_item.added","sequence_number":9,"output_index":3,"item":{"type":"message","id":"msg_2","role":"assistant","status":"in_progress","content":[]}}`},
		{Event: "response.output_text.delta", Data: `{"type":"response.output_text.delta","sequence_number":10,"item_id":"msg_2","output_index":3,"content_index":0,"delta":"等结果。","logprobs":[]}`},

		// completed: usage + finish reason via final Response.output
		{Event: "response.completed", Data: `{"type":"response.completed","sequence_number":11,"response":{"id":"resp_x","object":"response","model":"gpt-5","created_at":1700000000,"status":"completed","error":null,"incomplete_details":null,"instructions":null,"metadata":null,"output":[{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"想想看"}],"encrypted_content":"enc_xyz","status":"completed"},{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"先查天气：","annotations":[]}]},{"type":"function_call","id":"fc_1","call_id":"call_w","name":"weather","arguments":"{\"city\":\"BJ\"}","status":"completed"},{"type":"message","id":"msg_2","role":"assistant","status":"completed","content":[{"type":"output_text","text":"等结果。","annotations":[]}]}],"parallel_tool_calls":false,"temperature":1,"tool_choice":"auto","tools":[],"top_p":1,"usage":{"input_tokens":12,"output_tokens":8,"total_tokens":20,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":3}}}}`},
	}
	srv := testutil.AnthropicSSEServer(events)
	t.Cleanup(srv.Close)

	m := newResponsesModel(t, srv.URL, "gpt-5")
	req, _ := chat.NewRequest([]chat.Message{chat.NewUserMessage("查天气")})

	acc := chat.NewResponseAccumulator()
	for chunk, err := range m.Stream(t.Context(), req) {
		if err != nil {
			t.Fatalf("stream error: %v", err)
		}
		acc.AddChunk(chunk)
	}

	msg := acc.Response.Result.AssistantMessage
	if msg == nil {
		t.Fatal("AssistantMessage nil after accumulation")
	}
	if len(msg.Parts) != 4 {
		t.Fatalf("Parts len = %d; want 4", len(msg.Parts))
	}

	reasoning := msg.Parts[0].(*chat.ReasoningPart)
	if reasoning.Text != "想想看" {
		t.Errorf("reasoning text = %q", reasoning.Text)
	}
	if string(reasoning.Signature) != "enc_xyz" {
		t.Errorf("reasoning signature = %q; want enc_xyz", string(reasoning.Signature))
	}

	if msg.Parts[1].(*chat.TextPart).Text != "先查天气：" {
		t.Errorf("text1 = %q", msg.Parts[1].(*chat.TextPart).Text)
	}

	tc := msg.Parts[2].(*chat.ToolCallPart)
	if tc.ID != "call_w" || tc.Name != "weather" || tc.Arguments != `{"city":"BJ"}` {
		t.Errorf("tool call = %+v", tc)
	}

	if msg.Parts[3].(*chat.TextPart).Text != "等结果。" {
		t.Errorf("text2 = %q", msg.Parts[3].(*chat.TextPart).Text)
	}

	if acc.Response.Result.Metadata.FinishReason != chat.FinishReasonToolCalls {
		t.Errorf("FinishReason = %q", acc.Response.Result.Metadata.FinishReason)
	}
	if acc.Response.Metadata.Usage == nil || acc.Response.Metadata.Usage.PromptTokens != 12 {
		t.Errorf("usage = %+v", acc.Response.Metadata.Usage)
	}
}

func TestResponsesChatModel_Metadata(t *testing.T) {
	srv := testutil.JSONServer(http.StatusOK, "{}")
	t.Cleanup(srv.Close)
	m := newResponsesModel(t, srv.URL, "gpt-5")
	if m.Metadata().Provider != openai.Provider {
		t.Errorf("provider = %q; want %q", m.Metadata().Provider, openai.Provider)
	}
}
