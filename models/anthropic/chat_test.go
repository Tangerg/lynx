package anthropic_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"

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
		ApiKey:         model.NewApiKey("test-key"),
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
	if resp.Result.AssistantMessage.Text != "hello back" {
		t.Errorf("assistant text = %q; want %q", resp.Result.AssistantMessage.Text, "hello back")
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
			pieces = append(pieces, r.Result.AssistantMessage.Text)
		}
	}
	if !strings.Contains(strings.Join(pieces, ""), "hello world") {
		t.Errorf("accumulated text = %q; want to contain 'hello world'", strings.Join(pieces, ""))
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
