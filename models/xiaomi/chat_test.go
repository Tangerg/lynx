package xiaomi_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/xiaomi"
)

func TestChatModel(t *testing.T) {
	testutil.RunOpenAICompatChat(t, testutil.OpenAICompatChatContract{
		ProviderName: xiaomi.Provider,
		ModelID:      xiaomi.ModelV25,
		Build: func(t *testing.T, baseURL string) chat.Model {
			t.Helper()
			opts, err := chat.NewOptions(xiaomi.ModelV25)
			if err != nil {
				t.Fatalf("NewOptions: %v", err)
			}
			m, err := xiaomi.NewOpenAIChatModel(xiaomi.OpenAIChatModelConfig{
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
