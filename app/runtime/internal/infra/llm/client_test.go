package llm

import (
	"strings"
	"testing"
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
