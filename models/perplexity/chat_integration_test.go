//go:build integration

package perplexity_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/perplexity"
)

func TestChatModel_Integration(t *testing.T) {
	testutil.RunIntegrationChat(t, testutil.IntegrationChatProbe{
		Provider: "perplexity",
		Build: func(t *testing.T, key string) chat.Model {
			t.Helper()
			modelID, _ := testutil.LookupEnv("LYNX_TEST_PERPLEXITY_MODEL")
			if modelID == "" {
				modelID = perplexity.ModelSonar
			}
			opts, err := chat.NewOptions(modelID)
			if err != nil {
				t.Fatal(err)
			}
			m, err := perplexity.NewOpenAIChatModel(&perplexity.OpenAIChatModelConfig{
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
