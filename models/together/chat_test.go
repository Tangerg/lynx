package together_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/together"
)

func TestChatModel(t *testing.T) {
	testutil.RunOpenAICompatChat(t, testutil.OpenAICompatChatContract{
		ProviderName: together.Provider,
		ModelID:      together.ModelLlama33_70BInstructTurbo,
		Build: func(t *testing.T, baseURL string) chat.Model {
			t.Helper()
			opts, err := chat.NewOptions(together.ModelLlama33_70BInstructTurbo)
			if err != nil {
				t.Fatalf("NewOptions: %v", err)
			}
			m, err := together.NewOpenAIChatModel(&together.OpenAIChatModelConfig{
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
