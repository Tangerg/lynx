package xai_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/xai"
)

func TestChatModel(t *testing.T) {
	testutil.RunOpenAICompatChat(t, testutil.OpenAICompatChatContract{
		ProviderName: xai.Provider,
		ModelID:      xai.ModelGrok4,
		Build: func(t *testing.T, baseURL string) chat.Model {
			t.Helper()
			opts, err := chat.NewOptions(xai.ModelGrok4)
			if err != nil {
				t.Fatalf("NewOptions: %v", err)
			}
			m, err := xai.NewOpenAIChatModel(xai.OpenAIChatModelConfig{
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
