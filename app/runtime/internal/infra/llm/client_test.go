package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/models/deepseek"
)

// TestChatProviderCatalogSatisfiesConstructionContract holds the static catalog to its contract:
// every row builds, names a key env var, and the no-built-in-endpoint rows
// (the compatible endpoint providers and Azure) are flagged requiresBaseURL.
func TestChatProviderCatalogSatisfiesConstructionContract(t *testing.T) {
	for provider, profile := range chatProviderCatalog {
		if profile.build == nil {
			t.Errorf("provider %q: nil build func", provider)
		}
		if profile.apiKeyEnv == "" {
			t.Errorf("provider %q: empty apiKeyEnv", provider)
		}
	}

	// The compatible endpoint providers and Azure carry no built-in endpoint.
	for _, p := range []Provider{ProviderOpenAICompatible, ProviderAnthropicCompatible, ProviderAzureOpenAI} {
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
	if first.Output == nil || first.Output.Message == nil || len(first.Output.Message.Parts) != 2 || first.Output.Message.Parts[0].Kind != chat.PartReasoning {
		t.Fatalf("first response did not preserve reasoning: %#v", first)
	}

	second, err := client.Call(t.Context(), &chat.Request{Messages: []chat.Message{
		firstUser,
		*first.Output.Message,
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
	// A generic compatible endpoint has no catalog default — the model id is user-supplied.
	if ProviderOpenAICompatible.DefaultModel() != "" {
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
	if _, err := BuildClient(ClientSpec{Provider: ProviderOpenAICompatible, Model: "x", APIKey: "k"}); err == nil {
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
	if _, err := BuildClient(ClientSpec{Provider: ProviderOpenAICompatible, Model: "x", APIKey: "k", BaseURL: "https://gateway.example.com/v1"}); err != nil {
		t.Errorf("openai-compatible with base URL: %v", err)
	}
}

func TestDirectOpenAIUsesResponsesCountingWhileCompatibleRemainsChatCompletions(t *testing.T) {
	var countRequests atomic.Int32
	var responseRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/responses/input_tokens":
			countRequests.Add(1)
			_, _ = writer.Write([]byte(`{"object":"response.input_tokens","input_tokens":73}`))
		case "/responses":
			responseRequests.Add(1)
			_, _ = writer.Write([]byte(`{
  "id":"resp_scopeapp","object":"response","created_at":1,"status":"completed","model":"gpt-5.6-sol",
  "output":[{"type":"message","id":"msg_scopeapp","status":"completed","role":"assistant","content":[{"type":"output_text","text":"done","annotations":[]}]}],
  "parallel_tool_calls":false,"tools":[],
  "usage":{"input_tokens":73,"output_tokens":1,"total_tokens":74,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}
}`))
		default:
			t.Errorf("unexpected path %q", request.URL.Path)
			http.Error(writer, "unexpected path", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	direct, err := BuildClient(ClientSpec{
		Provider: ProviderOpenAI,
		Model:    defaultOpenAIModel,
		APIKey:   "test-key",
		BaseURL:  server.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !direct.SupportsInputTokenCounting() {
		t.Fatal("direct OpenAI client did not expose Responses input token counting")
	}
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("measure me")))
	if err != nil {
		t.Fatal(err)
	}
	count, err := direct.CountInputTokens(t.Context(), request)
	if err != nil || count != 73 || countRequests.Load() != 1 {
		t.Fatalf("direct CountInputTokens = %d, %v; requests=%d", count, err, countRequests.Load())
	}
	response, err := direct.Call(t.Context(), request)
	if err != nil || response.Text() != "done" || responseRequests.Load() != 1 {
		t.Fatalf("direct Responses Call = %#v, %v; requests=%d", response, err, responseRequests.Load())
	}

	compatible, err := BuildClient(ClientSpec{
		Provider: ProviderOpenAICompatible,
		Model:    "compatible-model",
		APIKey:   "test-key",
		BaseURL:  "https://gateway.example/v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if compatible.SupportsInputTokenCounting() {
		t.Fatal("OpenAI-compatible client advertised the native Responses count endpoint")
	}
}

func TestDirectAnthropicCountsWhileCompatibleDoesNotAssumeTheNativeEndpoint(t *testing.T) {
	var countRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !strings.HasSuffix(request.URL.Path, "/messages/count_tokens") {
			t.Errorf("path = %q, want /messages/count_tokens", request.URL.Path)
		}
		countRequests.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"input_tokens":61}`))
	}))
	t.Cleanup(server.Close)

	direct, err := BuildClient(ClientSpec{
		Provider: ProviderAnthropic,
		Model:    defaultAnthropicModel,
		APIKey:   "test-key",
		BaseURL:  server.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !direct.SupportsInputTokenCounting() {
		t.Fatal("direct Anthropic client did not expose Messages input token counting")
	}
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("measure me")))
	count, err := direct.CountInputTokens(t.Context(), request)
	if err != nil || count != 61 || countRequests.Load() != 1 {
		t.Fatalf("direct CountInputTokens = %d, %v; requests=%d", count, err, countRequests.Load())
	}

	compatible, err := BuildClient(ClientSpec{
		Provider: ProviderAnthropicCompatible,
		Model:    "compatible-model",
		APIKey:   "test-key",
		BaseURL:  "https://gateway.example/v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if compatible.SupportsInputTokenCounting() {
		t.Fatal("Anthropic-compatible client advertised the native Messages count endpoint")
	}
}
