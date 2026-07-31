package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/models/deepseek"
)

// TestProviderTable_Invariants holds the data-driven table to its contract:
// every row builds, names a key env var, and the no-built-in-endpoint rows
// (the generic passthroughs + Azure) are flagged requiresBaseURL.
func TestProviderTable_Invariants(t *testing.T) {
	for p, e := range providerInfo {
		if e.build == nil {
			t.Errorf("provider %q: nil build func", p)
		}
		if e.apiKeyEnv == "" {
			t.Errorf("provider %q: empty apiKeyEnv", p)
		}
	}

	// The generic passthroughs + Azure carry no built-in endpoint.
	for _, p := range []Provider{ProviderOpenAICompat, ProviderAnthropicCompat, ProviderAzureOpenAI} {
		if !p.RequiresBaseURL() {
			t.Errorf("provider %q must require a base URL", p)
		}
	}
	// A named vendor must NOT require one (it has a built-in endpoint).
	if ProviderAnthropic.RequiresBaseURL() {
		t.Error("anthropic must not require a base URL")
	}
}

func TestBuildClient_DeepSeekReasoningSurvivesOrdinarySecondTurn(t *testing.T) {
	var calls atomic.Int32
	var secondRequest struct {
		Messages []map[string]any `json:"messages"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		call := calls.Add(1)
		if call == 2 {
			if err := json.NewDecoder(request.Body).Decode(&secondRequest); err != nil {
				t.Errorf("decode second request: %v", err)
				http.Error(writer, "invalid request", http.StatusBadRequest)
				return
			}
		}
		writer.Header().Set("Content-Type", "application/json")
		if call == 1 {
			_, _ = writer.Write([]byte(`{"id":"first","model":"deepseek-v4-flash","choices":[{"index":0,"message":{"role":"assistant","reasoning_content":"private chain","content":"first answer"},"finish_reason":"stop"}]}`))
			return
		}
		_, _ = writer.Write([]byte(`{"id":"second","model":"deepseek-v4-flash","choices":[{"index":0,"message":{"role":"assistant","content":"second answer"},"finish_reason":"stop"}]}`))
	}))
	t.Cleanup(server.Close)

	client, err := BuildClient(ClientSpec{
		Provider: ProviderDeepSeek,
		Model:    deepseek.ModelV4Flash,
		APIKey:   "test-key",
		BaseURL:  server.URL,
	})
	if err != nil {
		t.Fatalf("BuildClient: %v", err)
	}
	firstUser := chat.NewUserMessage(chat.NewTextPart("first"))
	first, err := client.Call(t.Context(), &chat.Request{Messages: []chat.Message{firstUser}})
	if err != nil {
		t.Fatalf("first Call: %v", err)
	}
	if len(first.Choices) != 1 || first.Choices[0].Message == nil || len(first.Choices[0].Message.Parts) != 2 || first.Choices[0].Message.Parts[0].Kind != chat.PartReasoning {
		t.Fatalf("first response did not preserve reasoning: %#v", first)
	}

	second, err := client.Call(t.Context(), &chat.Request{Messages: []chat.Message{
		firstUser,
		*first.Choices[0].Message,
		chat.NewUserMessage(chat.NewTextPart("second")),
	}})
	if err != nil {
		t.Fatalf("second Call: %v", err)
	}
	if second.Text() != "second answer" {
		t.Fatalf("second response text = %q", second.Text())
	}
	assistant := findWireAssistant(t, secondRequest.Messages)
	if _, exists := assistant["reasoning_content"]; exists {
		t.Fatalf("ordinary prior turn replayed reasoning_content: %#v", assistant)
	}
}

func findWireAssistant(t *testing.T, messages []map[string]any) map[string]any {
	t.Helper()
	for _, message := range messages {
		if message["role"] == "assistant" {
			return message
		}
	}
	t.Fatal("assistant message not found")
	return nil
}

// TestQueries covers the table-reader API providers.list / config.Load lean on.
func TestQueries(t *testing.T) {
	if got := len(SupportedProviders()); got != 21 {
		t.Errorf("SupportedProviders = %d, want 21", got)
	}
	if !ProviderGroq.IsSupported() {
		t.Error("groq should be supported")
	}
	if Provider("nope").IsSupported() {
		t.Error("unknown provider should not be supported")
	}
	if ProviderAnthropic.DefaultModel() == "" {
		t.Error("anthropic should have a default model")
	}
	// A generic passthrough has no catalog default — the model id is user-supplied.
	if ProviderOpenAICompat.DefaultModel() != "" {
		t.Error("openai-compatible should have no default model")
	}
	if ProviderOpenAI.APIKeyEnv() != "OPENAI_API_KEY" {
		t.Errorf("openai key env = %q", ProviderOpenAI.APIKeyEnv())
	}
}

// TestBuildClient covers the construction guards + a successful build (the
// adapter constructs a client without touching the network — no key validation
// until a call is made).
func TestBuildClient(t *testing.T) {
	// Unknown provider → error.
	if _, err := BuildClient(ClientSpec{Provider: "nope", Model: "x"}); err == nil {
		t.Error("unknown provider must error")
	}
	// A requiresBaseURL provider without a base URL → error naming the gap.
	if _, err := BuildClient(ClientSpec{Provider: ProviderOpenAICompat, Model: "x", APIKey: "k"}); err == nil {
		t.Error("openai-compatible without base URL must error")
	} else if !strings.Contains(err.Error(), "base URL") {
		t.Errorf("error should mention the base URL: %v", err)
	}
	// A named vendor builds a non-nil client.
	c, err := BuildClient(ClientSpec{Provider: ProviderAnthropic, Model: "claude-3-5-haiku-20241022", APIKey: "test-key"})
	if err != nil || c == nil {
		t.Fatalf("build anthropic: client=%v err=%v", c, err)
	}
	// A requiresBaseURL provider WITH a base URL builds.
	if _, err := BuildClient(ClientSpec{Provider: ProviderOpenAICompat, Model: "x", APIKey: "k", BaseURL: "https://gateway.example.com/v1"}); err != nil {
		t.Errorf("openai-compatible with base URL: %v", err)
	}
}
