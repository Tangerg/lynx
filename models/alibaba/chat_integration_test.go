//go:build integration

package alibaba_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/alibaba"
	"github.com/Tangerg/lynx/models/internal/testutil"
)

func TestChatModel_Integration(t *testing.T) {
	testutil.RunIntegrationChat(t, testutil.IntegrationChatProbe{
		Provider: "alibaba",
		Build: func(t *testing.T, key string) chat.Model {
			t.Helper()
			modelID, _ := testutil.LookupEnv("LYNX_TEST_ALIBABA_MODEL")
			if modelID == "" {
				modelID = alibaba.ModelQwenTurbo
			}
			opts, err := chat.NewOptions(modelID)
			if err != nil {
				t.Fatal(err)
			}
			m, err := alibaba.NewOpenAIChatModel(&alibaba.OpenAIChatModelConfig{
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
