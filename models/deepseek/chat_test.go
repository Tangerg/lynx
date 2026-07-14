package deepseek_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/deepseek"
	"github.com/Tangerg/lynx/models/internal/testutil"
)

func TestChatModel(t *testing.T) {
	testutil.RunOpenAICompatChat(t, testutil.OpenAICompatChatContract{
		ProviderName: deepseek.Provider,
		ModelID:      deepseek.ModelChat,
		Build: func(t *testing.T, baseURL string) chat.Model {
			t.Helper()
			opts, err := chat.NewOptions(deepseek.ModelChat)
			if err != nil {
				t.Fatalf("NewOptions: %v", err)
			}
			m, err := deepseek.NewOpenAIChatModel(deepseek.OpenAIChatModelConfig{
				APIKey:         "test-key",
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
