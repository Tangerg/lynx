package moonshot_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/moonshot"
)

func TestAnthropicChatModel(t *testing.T) {
	testutil.RunAnthropicCompatChat(t, testutil.AnthropicCompatChatContract{
		ProviderName: moonshot.Provider,
		ModelID:      moonshot.ModelK2,
		Build: func(t *testing.T, baseURL string) chat.Model {
			t.Helper()
			opts, err := chat.NewOptions(moonshot.ModelK2)
			if err != nil {
				t.Fatalf("NewOptions: %v", err)
			}
			m, err := moonshot.NewAnthropicChatModel(&moonshot.AnthropicChatModelConfig{
				ApiKey:         model.NewApiKey("test-key"),
				DefaultOptions: opts,
				BaseURL:        baseURL,
			})
			if err != nil {
				t.Fatalf("NewAnthropicChatModel: %v", err)
			}
			return m
		},
	})
}
