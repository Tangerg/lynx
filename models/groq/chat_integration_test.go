//go:build integration

package groq_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/groq"
	"github.com/Tangerg/lynx/models/internal/testutil"
)

func TestChatModel_Integration(t *testing.T) {
	testutil.RunIntegrationChat(t, testutil.IntegrationChatProbe{
		Provider: "groq",
		Build: func(t *testing.T, key string) chat.Model {
			t.Helper()
			modelID, _ := testutil.LookupEnv("LYNX_TEST_GROQ_MODEL")
			if modelID == "" {
				modelID = groq.ModelLlama31_8BInstant
			}
			opts, err := chat.NewOptions(modelID)
			if err != nil {
				t.Fatal(err)
			}
			m, err := groq.NewOpenAIChatModel(groq.OpenAIChatModelConfig{
				APIKey:         model.NewAPIKey(key),
				DefaultOptions: opts,
			})
			if err != nil {
				t.Fatal(err)
			}
			return m
		},
	})
}
