package openai_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/modeltest"
	"github.com/Tangerg/lynx/models/protocol/openai"
)

func newResponsesModel(t *testing.T, baseURL, modelID string) *openai.ResponsesChat {
	t.Helper()
	m, err := openai.NewResponsesChat(openai.ChatConfig{
		APIKey:         "test-key",
		DefaultOptions: chat.Options{Model: modelID},
		BaseURL:        baseURL,
	})
	if err != nil {
		t.Fatalf("NewResponsesChat: %v", err)
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
	srv := modeltest.JSONServer(http.StatusOK, responsesInterleavedJSON, func(r *http.Request) {
		seenURL = r.URL.Path
	})
	t.Cleanup(srv.Close)

	m := newResponsesModel(t, srv.URL, "gpt-5")
	req, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("查天气")))

	resp, err := m.Call(t.Context(), req)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !strings.Contains(seenURL, "responses") {
		t.Errorf("URL = %q; want /v1/responses", seenURL)
	}

	msg := resp.Output.Message
	if msg == nil {
		t.Fatal("AssistantMessage is nil")
	}
	if len(msg.Parts) != 4 {
		t.Fatalf("Parts len = %d; want 4", len(msg.Parts))
	}
	wantKinds := []chat.PartKind{
		chat.PartReasoning,
		chat.PartText,
		chat.PartToolCall,
		chat.PartText,
	}
	for i, p := range msg.Parts {
		if p.Kind != wantKinds[i] {
			t.Errorf("Parts[%d].Kind = %s; want %s", i, p.Kind, wantKinds[i])
		}
	}

	reasoning := msg.Parts[0]
	if reasoning.Text != "想想看" {
		t.Errorf("reasoning text = %q", reasoning.Text)
	}
	if len(reasoning.Signature) == 0 || string(reasoning.Signature) == "enc_xyz" {
		t.Errorf("reasoning signature did not preserve the full reasoning item")
	}

	if msg.Parts[1].Text != "先查天气：" {
		t.Errorf("text[0] = %q", msg.Parts[1].Text)
	}
	if msg.Parts[3].Text != "等结果。" {
		t.Errorf("text[1] = %q", msg.Parts[3].Text)
	}

	tc := msg.Parts[2].ToolCall
	if tc.ID != "call_w" || tc.Name != "weather" || tc.Arguments != `{"city":"BJ"}` {
		t.Errorf("tool call = %+v", tc)
	}

	if resp.Output.FinishReason != chat.FinishReasonToolCalls {
		t.Errorf("FinishReason = %q; want tool_calls", resp.Output.FinishReason)
	}
	usage := resp.Metadata.Usage
	if usage.InputTokens != 12 || usage.OutputTokens != 8 {
		t.Errorf("usage tokens = %+v", usage)
	}
	if usage.ReasoningTokens == nil || *usage.ReasoningTokens != 3 {
		t.Errorf("reasoning tokens not surfaced: %+v", usage.ReasoningTokens)
	}
}

func TestResponsesChatModel_Stream_InterleavedDeltas(t *testing.T) {
	// Build the SSE event sequence by hand. Each event ships exactly one
	// part delta to lynx — reasoning → text → tool_call → text — and the
	// final response.completed carries usage + finish reason.
	events := []modeltest.AnthropicEvent{
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
	srv := modeltest.AnthropicSSEServer(events)
	t.Cleanup(srv.Close)

	m := newResponsesModel(t, srv.URL, "gpt-5")
	req, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("查天气")))

	acc := &chat.ResponseAccumulator{}
	for chunk, err := range m.Stream(t.Context(), req) {
		if err != nil {
			t.Fatalf("stream error: %v", err)
		}
		if err := acc.Add(chunk); err != nil {
			t.Fatalf("accumulate: %v", err)
		}
	}

	response := acc.Response()
	msg := response.Output.Message
	if msg == nil {
		t.Fatal("AssistantMessage nil after accumulation")
	}
	if len(msg.Parts) != 4 {
		t.Fatalf("Parts len = %d; want 4", len(msg.Parts))
	}

	reasoning := msg.Parts[0]
	if reasoning.Text != "想想看" {
		t.Errorf("reasoning text = %q", reasoning.Text)
	}
	if len(reasoning.Signature) == 0 || string(reasoning.Signature) == "enc_xyz" {
		t.Errorf("reasoning signature did not preserve the full reasoning item")
	}

	if msg.Parts[1].Text != "先查天气：" {
		t.Errorf("text1 = %q", msg.Parts[1].Text)
	}

	tc := msg.Parts[2].ToolCall
	if tc.ID != "call_w" || tc.Name != "weather" || tc.Arguments != `{"city":"BJ"}` {
		t.Errorf("tool call = %+v", tc)
	}

	if msg.Parts[3].Text != "等结果。" {
		t.Errorf("text2 = %q", msg.Parts[3].Text)
	}

	if response.Output.FinishReason != chat.FinishReasonToolCalls {
		t.Errorf("FinishReason = %q", response.Output.FinishReason)
	}
	if response.Metadata.Usage.InputTokens != 12 {
		t.Errorf("usage = %+v", response.Metadata.Usage)
	}
}

func TestResponsesChatReplaysProviderIssuedReasoningItem(t *testing.T) {
	requests := make([]map[string]any, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(writer, "invalid request", http.StatusBadRequest)
			return
		}
		requests = append(requests, body)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(responsesInterleavedJSON))
	}))
	t.Cleanup(server.Close)

	model := newResponsesModel(t, server.URL, "gpt-5")
	userMessage := chat.NewUserMessage(chat.NewTextPart("weather"))
	response, err := model.Call(t.Context(), &chat.Request{Messages: []chat.Message{userMessage}})
	if err != nil {
		t.Fatalf("first Call: %v", err)
	}
	if _, err := model.Call(t.Context(), &chat.Request{Messages: []chat.Message{
		userMessage,
		response.Output.Message.Clone(),
		chat.NewToolMessage(chat.ToolResult{ID: "call_w", Name: "weather", Result: "sunny"}),
	}}); err != nil {
		t.Fatalf("second Call: %v", err)
	}
	input := requests[1]["input"].([]any)
	var reasoning map[string]any
	for _, value := range input {
		item := value.(map[string]any)
		if item["type"] == "reasoning" {
			reasoning = item
			break
		}
	}
	if reasoning == nil {
		t.Fatalf("reasoning item missing from replay: %#v", input)
	}
	if reasoning["id"] != "rs_1" || reasoning["encrypted_content"] != "enc_xyz" || reasoning["status"] != "completed" {
		t.Fatalf("reasoning item identity changed: %#v", reasoning)
	}
	summary := reasoning["summary"].([]any)
	if len(summary) != 1 || summary[0].(map[string]any)["text"] != "想想看" {
		t.Fatalf("reasoning summary changed: %#v", summary)
	}
}

func TestResponsesChatRejectsUnsupportedOptions(t *testing.T) {
	srv := modeltest.JSONServer(http.StatusOK, "{}")
	t.Cleanup(srv.Close)
	m := newResponsesModel(t, srv.URL, "gpt-5")
	topK := int64(2)
	req, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("hello")))
	req.Options.TopK = &topK
	if _, err := m.Call(t.Context(), req); err == nil {
		t.Fatal("Call accepted unsupported top_k")
	}
}
