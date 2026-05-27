//go:build integration

package ollama_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/ollama"
)

func TestNativeChatModel_Integration(t *testing.T) {
	testutil.RunIntegrationChat(t, testutil.IntegrationChatProbe{
		Provider: "ollama",
		Build: func(t *testing.T, _ string) chat.Model {
			t.Helper()
			modelID, _ := testutil.LookupEnv("LYNX_TEST_OLLAMA_MODEL")
			if modelID == "" {
				modelID = "llama3.2"
			}
			baseURL, _ := testutil.LookupEnv("LYNX_TEST_OLLAMA_BASE_URL")
			opts, err := chat.NewOptions(modelID)
			if err != nil {
				t.Fatal(err)
			}
			m, err := ollama.NewNativeChatModel(ollama.NativeChatModelConfig{
				DefaultOptions: opts,
				BaseURL:        baseURL,
			})
			if err != nil {
				t.Fatal(err)
			}
			return m
		},
	})
}
