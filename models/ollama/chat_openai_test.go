package ollama_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/ollama"
)

// Ollama serves an OpenAI-compatible surface under /v1. The test
// model id is intentionally generic — Ollama doesn't export model
// constants since users pull arbitrary models locally.
const ollamaTestModel = "llama3.2"

func TestOpenAIChatModel(t *testing.T) {
	testutil.RunOpenAICompatChat(t, testutil.OpenAICompatChatContract{
		ProviderName: ollama.Provider,
		ModelID:      ollamaTestModel,
		Build: func(t *testing.T, baseURL string) chat.Model {
			t.Helper()
			// Ollama's OpenAI-compat path is BaseURL+/v1; the package
			// auto-appends that suffix. Strip it from the httptest URL
			// so the suffix gets re-added cleanly.
			base := strings.TrimSuffix(baseURL, "/v1")
			opts, err := chat.NewOptions(ollamaTestModel)
			if err != nil {
				t.Fatalf("NewOptions: %v", err)
			}
			m, err := ollama.NewOpenAIChatModel(ollama.OpenAIChatModelConfig{
				APIKey:         model.NewAPIKey("test-key"),
				DefaultOptions: opts,
				BaseURL:        base,
			})
			if err != nil {
				t.Fatalf("NewOpenAIChatModel: %v", err)
			}
			return m
		},
	})
}
