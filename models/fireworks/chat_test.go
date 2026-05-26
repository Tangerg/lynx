package fireworks_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/fireworks"
	"github.com/Tangerg/lynx/models/internal/testutil"
)

func TestChatModel(t *testing.T) {
	testutil.RunOpenAICompatChat(t, testutil.OpenAICompatChatContract{
		ProviderName: fireworks.Provider,
		ModelID:      fireworks.ModelLlamaV3p3_70BInstruct,
		Build: func(t *testing.T, baseURL string) chat.Model {
			t.Helper()
			opts, err := chat.NewOptions(fireworks.ModelLlamaV3p3_70BInstruct)
			if err != nil {
				t.Fatalf("NewOptions: %v", err)
			}
			m, err := fireworks.NewOpenAIChatModel(&fireworks.OpenAIChatModelConfig{
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
