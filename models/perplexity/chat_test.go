package perplexity_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/perplexity"
)

func TestChatModel(t *testing.T) {
	testutil.RunOpenAICompatChat(t, testutil.OpenAICompatChatContract{
		ProviderName: perplexity.Provider,
		ModelID:      perplexity.ModelSonar,
		Build: func(t *testing.T, baseURL string) chat.Model {
			t.Helper()
			opts, err := chat.NewOptions(perplexity.ModelSonar)
			if err != nil {
				t.Fatalf("NewOptions: %v", err)
			}
			m, err := perplexity.NewOpenAIChatModel(&perplexity.OpenAIChatModelConfig{
				ApiKey:         model.NewApiKey("test-key"),
				DefaultOptions: opts,
				BaseURL:        baseURL,
			})
			if err != nil {
				t.Fatalf("NewOpenAIChatModel: %v", err)
			}
			return m
		},
	})
}
