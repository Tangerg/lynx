package minimax_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/minimax"
)

func TestAnthropicChatModel(t *testing.T) {
	testutil.RunAnthropicCompatChat(t, testutil.AnthropicCompatChatContract{
		ProviderName: minimax.Provider,
		ModelID:      minimax.ModelM2,
		Build: func(t *testing.T, baseURL string) chat.Model {
			t.Helper()
			opts, err := chat.NewOptions(minimax.ModelM2)
			if err != nil {
				t.Fatalf("NewOptions: %v", err)
			}
			m, err := minimax.NewAnthropicChatModel(minimax.AnthropicChatModelConfig{
				APIKey:         model.NewAPIKey("test-key"),
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
