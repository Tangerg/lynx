//go:build integration

package huggingface_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/huggingface"
	"github.com/Tangerg/lynx/models/internal/testutil"
)

func TestChatModel_Integration(t *testing.T) {
	testutil.RunIntegrationChat(t, testutil.IntegrationChatProbe{
		Provider: "huggingface",
		Build: func(t *testing.T, key string) chat.Model {
			t.Helper()
			modelID, _ := testutil.LookupEnv("LYNX_TEST_HUGGINGFACE_MODEL")
			if modelID == "" {
				modelID = "meta-llama/Llama-3.3-70B-Instruct"
			}
			opts, err := chat.NewOptions(modelID)
			if err != nil {
				t.Fatal(err)
			}
			m, err := huggingface.NewOpenAIChatModel(huggingface.OpenAIChatModelConfig{
				APIKey:         key,
				DefaultOptions: opts,
			})
			if err != nil {
				t.Fatal(err)
			}
			return m
		},
	})
}
