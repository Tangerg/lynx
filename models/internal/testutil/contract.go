package testutil

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	openaisdk "github.com/openai/openai-go/v3"

	"github.com/Tangerg/lynx/core/model/chat"
)

// OpenAICompatChatContract is the standard mock-test contract for any
// OpenAI-compatible chat vendor (xai / groq / together / fireworks /
// perplexity / deepseek / moonshot / openrouter / azureopenai / ...).
// All these vendors share the same OpenAI wire format, so we exercise
// them through the same JSON / SSE surface.
//
// `Build` is the vendor-specific constructor — it receives the mock
// server's BaseURL and returns the wired chat.Model.
type OpenAICompatChatContract struct {
	// ProviderName is asserted against Model.Metadata().Provider.
	ProviderName string
	// ModelID drives the canned response body's model field.
	ModelID string
	// Build returns the model wired against the mock server.
	Build func(t *testing.T, baseURL string) chat.Model
}

// RunOpenAICompatChat runs the full mock contract — Call, Stream, and
// Metadata assertions. Vendor tests should `testutil.RunOpenAICompatChat(t, ...)`.
func RunOpenAICompatChat(t *testing.T, c OpenAICompatChatContract) {
	t.Helper()
	t.Run("Call_Mock", func(t *testing.T) { runOpenAICompatCall(t, c) })
	t.Run("Stream_Mock", func(t *testing.T) { runOpenAICompatStream(t, c) })
	t.Run("Metadata", func(t *testing.T) { runOpenAICompatMetadata(t, c) })
}

func runOpenAICompatCall(t *testing.T, c OpenAICompatChatContract) {
	completion := openaisdk.ChatCompletion{
		ID:      "chatcmpl-mock",
		Object:  "chat.completion",
		Model:   c.ModelID,
		Created: 1700000000,
		Choices: []openaisdk.ChatCompletionChoice{{
			Index:        0,
			FinishReason: "stop",
			Message: openaisdk.ChatCompletionMessage{
				Role:    "assistant",
				Content: "hello world",
			},
		}},
		Usage: openaisdk.CompletionUsage{
			PromptTokens:     5,
			CompletionTokens: 2,
			TotalTokens:      7,
		},
	}
	body, _ := json.Marshal(completion)

	var seenURL string
	srv := JSONServer(http.StatusOK, string(body), func(r *http.Request) {
		seenURL = r.URL.Path
	})
	t.Cleanup(srv.Close)

	m := c.Build(t, srv.URL)
	req, err := chat.NewRequest([]chat.Message{chat.NewUserMessage("hi")})
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	resp, err := m.Call(t.Context(), req)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !strings.HasSuffix(seenURL, "/chat/completions") {
		t.Errorf("URL = %q; want suffix /chat/completions", seenURL)
	}
	if resp.Result.AssistantMessage.JoinedText() != "hello world" {
		t.Errorf("text = %q; want %q", resp.Result.AssistantMessage.JoinedText(), "hello world")
	}
	if resp.Metadata.Usage == nil || resp.Metadata.Usage.PromptTokens != 5 {
		t.Errorf("usage = %+v; want PromptTokens=5", resp.Metadata.Usage)
	}
}

func runOpenAICompatStream(t *testing.T, c OpenAICompatChatContract) {
	chunks := []string{
		fmt.Sprintf(`{"id":"x","object":"chat.completion.chunk","model":%q,"choices":[{"index":0,"delta":{"role":"assistant"}}]}`, c.ModelID),
		fmt.Sprintf(`{"id":"x","object":"chat.completion.chunk","model":%q,"choices":[{"index":0,"delta":{"content":"hello"}}]}`, c.ModelID),
		fmt.Sprintf(`{"id":"x","object":"chat.completion.chunk","model":%q,"choices":[{"index":0,"delta":{"content":" world"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`, c.ModelID),
	}
	srv := OpenAISSEServer(chunks)
	t.Cleanup(srv.Close)

	m := c.Build(t, srv.URL)
	req, err := chat.NewRequest([]chat.Message{chat.NewUserMessage("hi")})
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	resps, err := Collect(m.Stream(t.Context(), req))
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if len(resps) != 3 {
		t.Fatalf("got %d chunks; want 3", len(resps))
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

	last := resps[len(resps)-1]
	if last.Metadata.Usage == nil || last.Metadata.Usage.PromptTokens != 3 {
		t.Errorf("last chunk usage = %+v; want PromptTokens=3", last.Metadata.Usage)
	}
}

func runOpenAICompatMetadata(t *testing.T, c OpenAICompatChatContract) {
	srv := JSONServer(http.StatusOK, "{}")
	t.Cleanup(srv.Close)
	m := c.Build(t, srv.URL)
	if got := m.Metadata().Provider; got != c.ProviderName {
		t.Errorf("provider = %q; want %q", got, c.ProviderName)
	}
}

// AnthropicCompatChatContract is the standard mock-test contract for
// any Anthropic-compatible chat vendor (anthropic / moonshot /
// openrouter / xiaomi / zhipu / minimax via NewAnthropicChatModel).
// All these vendors share Anthropic's /v1/messages wire format.
type AnthropicCompatChatContract struct {
	ProviderName string
	ModelID      string
	Build        func(t *testing.T, baseURL string) chat.Model
}

// RunAnthropicCompatChat runs the Anthropic-shape mock contract.
func RunAnthropicCompatChat(t *testing.T, c AnthropicCompatChatContract) {
	t.Helper()
	t.Run("Call_Mock", func(t *testing.T) { runAnthropicCompatCall(t, c) })
	t.Run("Stream_Mock", func(t *testing.T) { runAnthropicCompatStream(t, c) })
	t.Run("Metadata", func(t *testing.T) {
		srv := JSONServer(http.StatusOK, "{}")
		t.Cleanup(srv.Close)
		m := c.Build(t, srv.URL)
		if got := m.Metadata().Provider; got != c.ProviderName {
			t.Errorf("provider = %q; want %q", got, c.ProviderName)
		}
	})
}

func runAnthropicCompatCall(t *testing.T, c AnthropicCompatChatContract) {
	body := fmt.Sprintf(`{
  "id": "msg_test",
  "type": "message",
  "role": "assistant",
  "model": %q,
  "content": [{"type":"text","text":"hello back"}],
  "stop_reason": "end_turn",
  "usage": {"input_tokens": 4, "output_tokens": 2}
}`, c.ModelID)

	srv := JSONServer(http.StatusOK, body)
	t.Cleanup(srv.Close)

	m := c.Build(t, srv.URL)
	req, err := chat.NewRequest([]chat.Message{chat.NewUserMessage("hi")})
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := m.Call(t.Context(), req)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if resp.Result.AssistantMessage.JoinedText() != "hello back" {
		t.Errorf("text = %q; want %q", resp.Result.AssistantMessage.JoinedText(), "hello back")
	}
}

func runAnthropicCompatStream(t *testing.T, c AnthropicCompatChatContract) {
	events := []AnthropicEvent{
		{Event: "message_start", Data: fmt.Sprintf(`{"type":"message_start","message":{"id":"msg_x","type":"message","role":"assistant","model":%q,"content":[],"stop_reason":null,"usage":{"input_tokens":5,"output_tokens":0}}}`, c.ModelID)},
		{Event: "content_block_start", Data: `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		{Event: "content_block_delta", Data: `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`},
		{Event: "content_block_delta", Data: `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`},
		{Event: "content_block_stop", Data: `{"type":"content_block_stop","index":0}`},
		{Event: "message_delta", Data: `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`},
		{Event: "message_stop", Data: `{"type":"message_stop"}`},
	}
	srv := AnthropicSSEServer(events)
	t.Cleanup(srv.Close)

	m := c.Build(t, srv.URL)
	req, _ := chat.NewRequest([]chat.Message{chat.NewUserMessage("hi")})
	resps, err := Collect(m.Stream(t.Context(), req))
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

// IntegrationChatProbe is the standard real-API smoke probe for a chat
// model: Call returns text + Usage, Stream returns at least 2 chunks.
type IntegrationChatProbe struct {
	// Provider is the env-var prefix — e.g. "openai" → LYNX_TEST_OPENAI_KEY.
	Provider string
	// Build returns the model. Called only after a key is found.
	Build func(t *testing.T, key string) chat.Model
}

// RunIntegrationChat probes a real chat API: one Call + one Stream.
// The test is skipped when LYNX_TEST_<provider>_KEY is unset.
func RunIntegrationChat(t *testing.T, p IntegrationChatProbe) {
	t.Helper()
	key := RequireKey(t, p.Provider)
	m := p.Build(t, key)

	t.Run("Call", func(t *testing.T) {
		ctx, cancel := WithTimeout(t, 30*time.Second)
		defer cancel()
		req, err := chat.NewRequest([]chat.Message{
			chat.NewUserMessage("Reply with the single word: pong"),
		})
		if err != nil {
			t.Fatal(err)
		}
		resp, err := m.Call(ctx, req)
		if err != nil {
			t.Fatalf("Call: %v", err)
		}
		if resp.Result.AssistantMessage.JoinedText() == "" {
			t.Fatal("empty assistant text")
		}
	})

	t.Run("Stream", func(t *testing.T) {
		ctx, cancel := WithTimeout(t, 30*time.Second)
		defer cancel()
		req, err := chat.NewRequest([]chat.Message{
			chat.NewUserMessage("Count from 1 to 5, comma-separated."),
		})
		if err != nil {
			t.Fatal(err)
		}
		resps, err := Collect(m.Stream(ctx, req))
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}
		if len(resps) < 2 {
			t.Fatalf("got %d chunks; want at least 2", len(resps))
		}
	})
}
