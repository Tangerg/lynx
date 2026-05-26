package zhipu_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/zhipu"
)

func TestAnthropicChatModel(t *testing.T) {
	testutil.RunAnthropicCompatChat(t, testutil.AnthropicCompatChatContract{
		ProviderName: zhipu.Provider,
		ModelID:      zhipu.ModelGLM46,
		Build: func(t *testing.T, baseURL string) chat.Model {
			t.Helper()
			opts, err := chat.NewOptions(zhipu.ModelGLM46)
			if err != nil {
				t.Fatalf("NewOptions: %v", err)
			}
			m, err := zhipu.NewAnthropicChatModel(&zhipu.AnthropicChatModelConfig{
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
