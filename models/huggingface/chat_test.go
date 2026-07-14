package huggingface_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/huggingface"
	"github.com/Tangerg/lynx/models/internal/testutil"
)

// huggingface routes to multiple inference providers — no exported
// model constants. Use a generic test model id.
const hfTestModel = "meta-llama/Llama-3.3-70B-Instruct"

func TestChatModel(t *testing.T) {
	testutil.RunOpenAICompatChat(t, testutil.OpenAICompatChatContract{
		ProviderName: huggingface.Provider,
		ModelID:      hfTestModel,
		Build: func(t *testing.T, baseURL string) chat.Model {
			t.Helper()
			opts, err := chat.NewOptions(hfTestModel)
			if err != nil {
				t.Fatalf("NewOptions: %v", err)
			}
			m, err := huggingface.NewOpenAIChatModel(huggingface.OpenAIChatModelConfig{
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
