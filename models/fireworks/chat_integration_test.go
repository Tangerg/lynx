//go:build integration

package fireworks_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/fireworks"
	"github.com/Tangerg/lynx/models/internal/testutil"
)

func TestChatModel_Integration(t *testing.T) {
	testutil.RunIntegrationChat(t, testutil.IntegrationChatProbe{
		Provider: "fireworks",
		Build: func(t *testing.T, key string) chat.Model {
			t.Helper()
			modelID, _ := testutil.LookupEnv("LYNX_TEST_FIREWORKS_MODEL")
			if modelID == "" {
				modelID = fireworks.ModelLlamaV3p3_70BInstruct
			}
			opts, err := chat.NewOptions(modelID)
			if err != nil {
				t.Fatal(err)
			}
			m, err := fireworks.NewOpenAIChatModel(&fireworks.OpenAIChatModelConfig{
				ApiKey:         model.NewApiKey(key),
				DefaultOptions: opts,
			})
			if err != nil {
				t.Fatal(err)
			}
			return m
		},
	})
}
