package openrouter_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/openrouter"
)

const orAnthropicTestModel = "anthropic/claude-3-5-sonnet"

func TestAnthropicChatModel(t *testing.T) {
	testutil.RunAnthropicCompatChat(t, testutil.AnthropicCompatChatContract{
		ProviderName: openrouter.Provider,
		ModelID:      orAnthropicTestModel,
		Build: func(t *testing.T, baseURL string) chat.Model {
			t.Helper()
			opts, err := chat.NewOptions(orAnthropicTestModel)
			if err != nil {
				t.Fatalf("NewOptions: %v", err)
			}
			m, err := openrouter.NewAnthropicChatModel(openrouter.AnthropicChatModelConfig{
				APIKey:         "test-key",
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
