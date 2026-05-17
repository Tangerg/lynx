package xai_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/openai"
	"github.com/Tangerg/lynx/models/xai"
)

func newChatModel(t *testing.T, baseURL string) *openai.ChatModel {
	t.Helper()
	opts, err := chat.NewOptions(xai.ModelGrok4)
	if err != nil {
		t.Fatalf("NewOptions: %v", err)
	}
	m, err := xai.NewOpenAIChatModel(&xai.OpenAIChatModelConfig{
		ApiKey:         model.NewApiKey("test-key"),
		DefaultOptions: opts,
		BaseURL:        baseURL,
		RequestOptions: []option.RequestOption{},
	})
	if err != nil {
		t.Fatalf("NewOpenAIChatModel: %v", err)
	}
	return m
}

func TestChatModel_Call_Mock(t *testing.T) {
	completion := openaisdk.ChatCompletion{
		ID: "x1", Object: "chat.completion", Model: xai.ModelGrok4,
		Choices: []openaisdk.ChatCompletionChoice{{
			Index: 0, FinishReason: "stop",
			Message: openaisdk.ChatCompletionMessage{Role: "assistant", Content: "grok says hi"},
		}},
		Usage: openaisdk.CompletionUsage{PromptTokens: 4, CompletionTokens: 3, TotalTokens: 7},
	}
	body, _ := json.Marshal(completion)

	var seenURL string
	srv := testutil.JSONServer(http.StatusOK, string(body), func(r *http.Request) {
		seenURL = r.URL.Path
	})
	t.Cleanup(srv.Close)

	m := newChatModel(t, srv.URL)
	req, _ := chat.NewRequest([]chat.Message{chat.NewUserMessage("hi")})

	resp, err := m.Call(t.Context(), req)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !strings.HasSuffix(seenURL, "/chat/completions") {
		t.Errorf("URL = %q; want /chat/completions suffix", seenURL)
	}
	if resp.Result.AssistantMessage.Text != "grok says hi" {
		t.Errorf("text = %q", resp.Result.AssistantMessage.Text)
	}
	if m.Metadata().Provider != xai.Provider {
		t.Errorf("provider = %q; want %q", m.Metadata().Provider, xai.Provider)
	}
}

func TestChatModel_Stream_Mock(t *testing.T) {
	chunks := []string{
		`{"id":"x","object":"chat.completion.chunk","model":"grok-4","choices":[{"index":0,"delta":{"content":"hello"}}]}`,
		`{"id":"x","object":"chat.completion.chunk","model":"grok-4","choices":[{"index":0,"delta":{"content":" grok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":2,"total_tokens":4}}`,
	}
	srv := testutil.OpenAISSEServer(chunks)
	t.Cleanup(srv.Close)

	m := newChatModel(t, srv.URL)
	req, _ := chat.NewRequest([]chat.Message{chat.NewUserMessage("hi")})

	resps, err := testutil.Collect(m.Stream(t.Context(), req))
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if len(resps) != 2 {
		t.Fatalf("got %d chunks; want 2", len(resps))
	}
}
