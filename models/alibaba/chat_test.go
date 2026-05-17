package alibaba_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/alibaba"
	"github.com/Tangerg/lynx/models/internal/testutil"
)

func TestChatModel(t *testing.T) {
	testutil.RunOpenAICompatChat(t, testutil.OpenAICompatChatContract{
		ProviderName: alibaba.Provider,
		ModelID:      alibaba.ModelQwenTurbo,
		Build: func(t *testing.T, baseURL string) chat.Model {
			t.Helper()
			opts, err := chat.NewOptions(alibaba.ModelQwenTurbo)
			if err != nil {
				t.Fatalf("NewOptions: %v", err)
			}
			m, err := alibaba.NewOpenAIChatModel(&alibaba.OpenAIChatModelConfig{
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
