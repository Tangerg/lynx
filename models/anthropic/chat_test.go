package anthropic_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/anthropic"
	"github.com/Tangerg/lynx/models/internal/testutil"
)

func newChatModel(t *testing.T, baseURL, modelID string) *anthropic.ChatModel {
	t.Helper()
	opts, err := chat.NewOptions(modelID)
	if err != nil {
		t.Fatalf("NewOptions: %v", err)
	}
	m, err := anthropic.NewChatModel(&anthropic.ChatModelConfig{
		APIKey:         model.NewAPIKey("test-key"),
		DefaultOptions: opts,
		RequestOptions: []option.RequestOption{option.WithBaseURL(baseURL)},
	})
	if err != nil {
		t.Fatalf("NewChatModel: %v", err)
	}
	return m
}

// anthropicResponseJSON is a minimal /v1/messages response — Message
// resource shape per https://docs.anthropic.com/en/api/messages.
const anthropicResponseJSON = `{
  "id": "msg_abc",
  "type": "message",
  "role": "assistant",
  "model": "claude-3-5-sonnet-20241022",
  "content": [{"type":"text","text":"hello back"}],
  "stop_reason": "end_turn",
  "usage": {"input_tokens": 4, "output_tokens": 2}
}`

func TestChatModel_Call_Mock(t *testing.T) {
	var seenAuth, seenVersion, seenURL string
	srv := testutil.JSONServer(http.StatusOK, anthropicResponseJSON, func(r *http.Request) {
		seenAuth = r.Header.Get("x-api-key")
		seenVersion = r.Header.Get("anthropic-version")
		seenURL = r.URL.Path
	})
	t.Cleanup(srv.Close)

	m := newChatModel(t, srv.URL, "claude-3-5-sonnet-20241022")
	req, _ := chat.NewRequest([]chat.Message{chat.NewUserMessage("hi")})

	resp, err := m.Call(t.Context(), req)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if seenAuth != "test-key" {
		t.Errorf("x-api-key = %q; want test-key", seenAuth)
	}
	if seenVersion == "" {
		t.Errorf("anthropic-version header was not sent")
	}
	if !strings.Contains(seenURL, "messages") {
		t.Errorf("URL = %q; want /v1/messages", seenURL)
	}
	if resp.Result.AssistantMessage.JoinedText() != "hello back" {
		t.Errorf("assistant text = %q; want %q", resp.Result.AssistantMessage.JoinedText(), "hello back")
	}
	if resp.Metadata.Usage == nil || resp.Metadata.Usage.PromptTokens != 4 || resp.Metadata.Usage.CompletionTokens != 2 {
		t.Errorf("usage = %+v; want PromptTokens=4 CompletionTokens=2", resp.Metadata.Usage)
	}
}

func TestChatModel_Stream_Mock(t *testing.T) {
	events := []testutil.AnthropicEvent{
		{Event: "message_start", Data: `{"type":"message_start","message":{"id":"msg_x","type":"message","role":"assistant","model":"claude-3-5-sonnet-20241022","content":[],"stop_reason":null,"usage":{"input_tokens":5,"output_tokens":0}}}`},
		{Event: "content_block_start", Data: `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		{Event: "content_block_delta", Data: `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`},
		{Event: "content_block_delta", Data: `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`},
		{Event: "content_block_stop", Data: `{"type":"content_block_stop","index":0}`},
		{Event: "message_delta", Data: `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":2}}`},
		{Event: "message_stop", Data: `{"type":"message_stop"}`},
	}
	srv := testutil.AnthropicSSEServer(events)
	t.Cleanup(srv.Close)

	m := newChatModel(t, srv.URL, "claude-3-5-sonnet-20241022")
	req, _ := chat.NewRequest([]chat.Message{chat.NewUserMessage("hi")})

	resps, err := testutil.Collect(m.Stream(t.Context(), req))
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if len(resps) == 0 {
		t.Fatal("got 0 chunks")
	}

	var pieces []string
	for _, r := range resps {
		if r.Result != nil && r.Result.AssistantMessage != nil {
			pieces = append(pieces, r.Result.AssistantMessage.JoinedText())
		}
	}
	if !strings.Contains(strings.Join(pieces, ""), "hello world") {
		t.Errorf("accumulated text = %q; want to contain 'hello world'", strings.Join(pieces, ""))
	}
}

// TestChatModel_Stream_InterleavedThinkingTextToolUse exercises the
// canonical Claude "thinking → text → tool_use → text → tool_use →
// text" interleaving — the headline use case the Parts data model
// exists for. Verifies:
//   1. Parts arrive in emission order (no flattening)
//   2. ReasoningPart carries Signature
//   3. Two ToolCallParts are distinct (different IDs)
//   4. tool_use arguments accumulate across input_json_delta chunks
//   5. FinishReason reflects stop_reason=tool_use
//   6. Usage metadata bubbles through
func TestChatModel_Stream_InterleavedThinkingTextToolUse(t *testing.T) {
	events := []testutil.AnthropicEvent{
		{Event: "message_start", Data: `{"type":"message_start","message":{"id":"msg_x","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[],"stop_reason":null,"usage":{"input_tokens":12,"output_tokens":0}}}`},

		// thinking block (idx 0)
		{Event: "content_block_start", Data: `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`},
		{Event: "content_block_delta", Data: `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"用户问天气和日历..."}}`},
		{Event: "content_block_delta", Data: `{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig_abc"}}`},
		{Event: "content_block_stop", Data: `{"type":"content_block_stop","index":0}`},

		// text block "好的，先查天气：" (idx 1)
		{Event: "content_block_start", Data: `{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`},
		{Event: "content_block_delta", Data: `{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"好的，先查天气："}}`},
		{Event: "content_block_stop", Data: `{"type":"content_block_stop","index":1}`},

		// tool_use weather (idx 2) — arguments arrive as 2 partial JSON deltas
		{Event: "content_block_start", Data: `{"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"tu_1","name":"weather","input":{}}}`},
		{Event: "content_block_delta", Data: `{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"city\":"}}`},
		{Event: "content_block_delta", Data: `{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"\"北京\"}"}}`},
		{Event: "content_block_stop", Data: `{"type":"content_block_stop","index":2}`},

		// text block "再查日历：" (idx 3)
		{Event: "content_block_start", Data: `{"type":"content_block_start","index":3,"content_block":{"type":"text","text":""}}`},
		{Event: "content_block_delta", Data: `{"type":"content_block_delta","index":3,"delta":{"type":"text_delta","text":"再查日历："}}`},
		{Event: "content_block_stop", Data: `{"type":"content_block_stop","index":3}`},

		// tool_use calendar (idx 4)
		{Event: "content_block_start", Data: `{"type":"content_block_start","index":4,"content_block":{"type":"tool_use","id":"tu_2","name":"calendar","input":{}}}`},
		{Event: "content_block_delta", Data: `{"type":"content_block_delta","index":4,"delta":{"type":"input_json_delta","partial_json":"{\"date\":\"tomorrow\"}"}}`},
		{Event: "content_block_stop", Data: `{"type":"content_block_stop","index":4}`},

		// final text "等结果回来再总结。" (idx 5)
		{Event: "content_block_start", Data: `{"type":"content_block_start","index":5,"content_block":{"type":"text","text":""}}`},
		{Event: "content_block_delta", Data: `{"type":"content_block_delta","index":5,"delta":{"type":"text_delta","text":"等结果回来再总结。"}}`},
		{Event: "content_block_stop", Data: `{"type":"content_block_stop","index":5}`},

		{Event: "message_delta", Data: `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":42}}`},
		{Event: "message_stop", Data: `{"type":"message_stop"}`},
	}
	srv := testutil.AnthropicSSEServer(events)
	t.Cleanup(srv.Close)

	m := newChatModel(t, srv.URL, "claude-sonnet-4-5")
	req, _ := chat.NewRequest([]chat.Message{chat.NewUserMessage("查天气和日历")})

	// Accumulate via Stream → ResponseAccumulator. This replicates what
	// chat.Client does internally on the call path.
	acc := chat.NewResponseAccumulator()
	for chunk, err := range m.Stream(t.Context(), req) {
		if err != nil {
			t.Fatalf("stream error: %v", err)
		}
		acc.AddChunk(chunk)
	}

	final := acc.Response.Result.AssistantMessage
	if final == nil {
		t.Fatal("nil AssistantMessage after accumulation")
	}

	// 1. Parts arrive in emission order: reasoning, text, tool_call,
	//    text, tool_call, text — 6 ordered parts.
	if len(final.Parts) != 6 {
		t.Fatalf("Parts len = %d; want 6", len(final.Parts))
	}
	wantKinds := []chat.PartKind{
		chat.PartKindReasoning,
		chat.PartKindText,
		chat.PartKindToolCall,
		chat.PartKindText,
		chat.PartKindToolCall,
		chat.PartKindText,
	}
	for i, p := range final.Parts {
		if p.Kind() != wantKinds[i] {
			t.Errorf("Parts[%d].Kind() = %s; want %s", i, p.Kind(), wantKinds[i])
		}
	}

	// 2. ReasoningPart carries text + signature.
	reasoning := final.Parts[0].(*chat.ReasoningPart)
	if reasoning.Text != "用户问天气和日历..." {
		t.Errorf("reasoning text = %q", reasoning.Text)
	}
	if string(reasoning.Signature) != "sig_abc" {
		t.Errorf("reasoning signature = %q; want sig_abc", reasoning.Signature)
	}

	// 3. Three TextParts in order with the right bodies.
	textParts := []string{
		final.Parts[1].(*chat.TextPart).Text,
		final.Parts[3].(*chat.TextPart).Text,
		final.Parts[5].(*chat.TextPart).Text,
	}
	wantTexts := []string{"好的，先查天气：", "再查日历：", "等结果回来再总结。"}
	for i, got := range textParts {
		if got != wantTexts[i] {
			t.Errorf("text[%d] = %q; want %q", i, got, wantTexts[i])
		}
	}

	// 4. Two distinct ToolCallParts with accumulated arguments.
	tool1 := final.Parts[2].(*chat.ToolCallPart)
	tool2 := final.Parts[4].(*chat.ToolCallPart)
	if tool1.ID != "tu_1" || tool1.Name != "weather" {
		t.Errorf("tool1 ID/name = %q/%q", tool1.ID, tool1.Name)
	}
	if tool1.Arguments != `{"city":"北京"}` {
		t.Errorf("tool1 args = %q; want %q", tool1.Arguments, `{"city":"北京"}`)
	}
	if tool2.ID != "tu_2" || tool2.Name != "calendar" {
		t.Errorf("tool2 ID/name = %q/%q", tool2.ID, tool2.Name)
	}
	if tool2.Arguments != `{"date":"tomorrow"}` {
		t.Errorf("tool2 args = %q", tool2.Arguments)
	}

	// 5. FinishReason = tool_calls.
	if acc.Response.Result.Metadata.FinishReason != chat.FinishReasonToolCalls {
		t.Errorf("FinishReason = %q; want tool_calls", acc.Response.Result.Metadata.FinishReason)
	}

	// 6. Usage bubbled through.
	if acc.Response.Metadata == nil || acc.Response.Metadata.Usage == nil {
		t.Fatal("usage missing")
	}
	if acc.Response.Metadata.Usage.CompletionTokens != 42 {
		t.Errorf("CompletionTokens = %d; want 42", acc.Response.Metadata.Usage.CompletionTokens)
	}

	// 7. Derived helpers correctly walk Parts.
	if final.JoinedText() != "好的，先查天气：再查日历：等结果回来再总结。" {
		t.Errorf("JoinedText = %q", final.JoinedText())
	}
	if final.JoinedReasoning() != "用户问天气和日历..." {
		t.Errorf("JoinedReasoning = %q", final.JoinedReasoning())
	}
	if !final.HasToolCalls() {
		t.Error("HasToolCalls should be true")
	}
	if !final.HasReasoning() {
		t.Error("HasReasoning should be true")
	}

	calls := final.CollectToolCalls()
	if len(calls) != 2 {
		t.Errorf("CollectToolCalls len = %d; want 2", len(calls))
	}
}

// TestChatModel_RoundTrip_ParseAndReassemble verifies that an
// accumulated AssistantMessage can be re-fed to the same adapter as a
// history message and emerges as semantically equivalent wire blocks.
// This is the lossless property §6.6.1 of the design doc claims for
// Anthropic.
func TestChatModel_RoundTrip_ParseAndReassemble(t *testing.T) {
	// Build an assistant message by hand with the same shape the
	// interleaved-stream test produced.
	original := chat.NewAssistantMessage(chat.MessageParams{
		Parts: []chat.OutputPart{
			&chat.ReasoningPart{Text: "thinking step", Signature: []byte("sig_xyz")},
			&chat.TextPart{Text: "Step 1: "},
			&chat.ToolCallPart{ID: "tu_a", Name: "search", Arguments: `{"q":"x"}`},
			&chat.TextPart{Text: "Step 2."},
		},
	})

	// Set up a server that echoes back what it received so we can
	// inspect the messages wire payload.
	var seenBody []byte
	srv := testutil.JSONServer(http.StatusOK, anthropicResponseJSON, func(r *http.Request) {
		seenBody, _ = io.ReadAll(r.Body)
	})
	t.Cleanup(srv.Close)

	m := newChatModel(t, srv.URL, "claude-sonnet-4-5")
	req, _ := chat.NewRequest([]chat.Message{
		chat.NewUserMessage("hello"),
		original,
		chat.NewUserMessage("continue"),
	})
	_, _ = m.Call(t.Context(), req)

	body := string(seenBody)

	// 1. Thinking block with signature is preserved verbatim.
	if !strings.Contains(body, `"type":"thinking"`) {
		t.Errorf("thinking block missing in wire body: %s", body)
	}
	if !strings.Contains(body, `"signature":"sig_xyz"`) {
		t.Errorf("signature missing in wire body: %s", body)
	}
	if !strings.Contains(body, `"thinking":"thinking step"`) {
		t.Errorf("thinking text missing: %s", body)
	}

	// 2. Tool use block is emitted with the right ID + name + input.
	if !strings.Contains(body, `"type":"tool_use"`) {
		t.Errorf("tool_use block missing: %s", body)
	}
	if !strings.Contains(body, `"id":"tu_a"`) {
		t.Errorf("tool_use id missing: %s", body)
	}
	if !strings.Contains(body, `"name":"search"`) {
		t.Errorf("tool_use name missing: %s", body)
	}

	// 3. Both text blocks appear in their original order.
	idxStep1 := strings.Index(body, `"Step 1: "`)
	idxStep2 := strings.Index(body, `"Step 2."`)
	if idxStep1 < 0 || idxStep2 < 0 {
		t.Fatalf("text blocks missing: step1=%d step2=%d body=%s", idxStep1, idxStep2, body)
	}
	if idxStep1 >= idxStep2 {
		t.Errorf("text blocks reordered: Step 1 (%d) should precede Step 2 (%d)", idxStep1, idxStep2)
	}

	// 4. Reasoning precedes text in the wire (Anthropic ordering rule:
	//    thinking blocks must appear before text in a turn).
	idxThinking := strings.Index(body, `"type":"thinking"`)
	if idxThinking < 0 || idxStep1 < 0 {
		t.Fatalf("ordering check setup failed")
	}
	if idxThinking >= idxStep1 {
		t.Errorf("thinking block (%d) should precede first text (%d)", idxThinking, idxStep1)
	}
}

// TestChatModel_Call_PreservesPreStagedSystemForCaching verifies the
// prompt-caching contract: when the caller pre-stages
// anthropicsdk.MessageNewParams in Options.Extra (typically to attach
// cache_control to a system block or trailing tool definition), the
// adapter must NOT overwrite those pre-staged fields when building
// the wire request. Lynx-derived blocks (from req.Messages /
// req.Tools) are appended after the pre-staged content so the
// caller's cache_control breakpoint keeps its leading position.
func TestChatModel_Call_PreservesPreStagedSystemForCaching(t *testing.T) {
	var seenBody []byte
	srv := testutil.JSONServer(http.StatusOK, anthropicResponseJSON, func(r *http.Request) {
		seenBody, _ = io.ReadAll(r.Body)
	})
	t.Cleanup(srv.Close)

	m := newChatModel(t, srv.URL, "claude-3-5-sonnet-20241022")

	// Pre-stage a system block carrying cache_control + a tool block
	// also carrying cache_control. This is the canonical prompt-caching
	// shape per https://docs.anthropic.com/en/docs/build-with-claude/prompt-caching.
	staged := &anthropicsdk.MessageNewParams{
		System: []anthropicsdk.TextBlockParam{
			{
				Text:         "static cached instructions",
				CacheControl: anthropicsdk.NewCacheControlEphemeralParam(),
			},
		},
		Tools: []anthropicsdk.ToolUnionParam{
			{
				OfTool: &anthropicsdk.ToolParam{
					Name:        "search",
					Description: param.NewOpt("static cached tool"),
					InputSchema: anthropicsdk.ToolInputSchemaParam{
						ExtraFields: map[string]any{
							"properties": map[string]any{
								"q": map[string]any{"type": "string"},
							},
						},
					},
					CacheControl: anthropicsdk.NewCacheControlEphemeralParam(),
				},
			},
		},
	}

	opts, _ := chat.NewOptions("claude-3-5-sonnet-20241022")
	opts.Set(anthropic.OptionsKey, staged)

	req, _ := chat.NewRequest([]chat.Message{
		chat.NewSystemMessage("runtime instructions"),
		chat.NewUserMessage("hi"),
	})
	req.Options = opts

	if _, err := m.Call(t.Context(), req); err != nil {
		t.Fatalf("Call: %v", err)
	}

	body := string(seenBody)

	// 1. Pre-staged cached system block must survive.
	if !strings.Contains(body, "static cached instructions") {
		t.Fatalf("pre-staged system text was dropped: %s", body)
	}
	if !strings.Contains(body, `"cache_control":{"type":"ephemeral"}`) {
		t.Fatalf("cache_control was dropped: %s", body)
	}

	// 2. Lynx-derived runtime system text must also appear — appended
	//    after the cached block.
	if !strings.Contains(body, "runtime instructions") {
		t.Fatalf("runtime system message missing: %s", body)
	}
	idxStatic := strings.Index(body, "static cached instructions")
	idxRuntime := strings.Index(body, "runtime instructions")
	if idxStatic >= idxRuntime {
		t.Fatalf("pre-staged block must precede runtime block (static=%d, runtime=%d)", idxStatic, idxRuntime)
	}

	// 3. Pre-staged tool with cache_control must survive.
	idxStaticTool := strings.Index(body, `"name":"search"`)
	if idxStaticTool < 0 {
		t.Fatalf("pre-staged tool missing: %s", body)
	}
}

func TestChatModel_Metadata(t *testing.T) {
	srv := testutil.JSONServer(http.StatusOK, "{}")
	t.Cleanup(srv.Close)
	m := newChatModel(t, srv.URL, "claude-3-5-sonnet-20241022")
	if m.Metadata().Provider != anthropic.Provider {
		t.Errorf("provider = %q; want %q", m.Metadata().Provider, anthropic.Provider)
	}
}
