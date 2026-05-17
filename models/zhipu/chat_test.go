package zhipu_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/zhipu"
)

func TestChatModel(t *testing.T) {
	testutil.RunOpenAICompatChat(t, testutil.OpenAICompatChatContract{
		ProviderName: zhipu.Provider,
		ModelID:      zhipu.ModelGLM4Flash,
		Build: func(t *testing.T, baseURL string) chat.Model {
			t.Helper()
			opts, err := chat.NewOptions(zhipu.ModelGLM4Flash)
			if err != nil {
				t.Fatalf("NewOptions: %v", err)
			}
			m, err := zhipu.NewOpenAIChatModel(&zhipu.OpenAIChatModelConfig{
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
