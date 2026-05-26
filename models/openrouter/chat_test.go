package openrouter_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/openrouter"
)

// openrouter has no exported model constants — users pass strings.
const orTestModel = "openai/gpt-4o-mini"

func TestChatModel(t *testing.T) {
	testutil.RunOpenAICompatChat(t, testutil.OpenAICompatChatContract{
		ProviderName: openrouter.Provider,
		ModelID:      orTestModel,
		Build: func(t *testing.T, baseURL string) chat.Model {
			t.Helper()
			opts, err := chat.NewOptions(orTestModel)
			if err != nil {
				t.Fatalf("NewOptions: %v", err)
			}
			m, err := openrouter.NewOpenAIChatModel(&openrouter.OpenAIChatModelConfig{
				APIKey:         model.NewAPIKey("test-key"),
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
