package groq_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/groq"
	"github.com/Tangerg/lynx/models/internal/testutil"
)

func TestChatModel(t *testing.T) {
	testutil.RunOpenAICompatChat(t, testutil.OpenAICompatChatContract{
		ProviderName: groq.Provider,
		ModelID:      groq.ModelLlama33_70BVersatile,
		Build: func(t *testing.T, baseURL string) chat.Model {
			t.Helper()
			opts, err := chat.NewOptions(groq.ModelLlama33_70BVersatile)
			if err != nil {
				t.Fatalf("NewOptions: %v", err)
			}
			m, err := groq.NewOpenAIChatModel(groq.OpenAIChatModelConfig{
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
