//go:build integration

package vertexai_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/vertexai"
)

// Vertex AI authenticates via Application Default Credentials, not an
// API key. The "LYNX_TEST_VERTEXAI_KEY" sentinel + LYNX_TEST_GCP_PROJECT
// / LYNX_TEST_GCP_LOCATION gate the test.
func TestChatModel_Integration(t *testing.T) {
	testutil.RequireKey(t, "vertexai")
	project := testutil.RequireEnv(t, "LYNX_TEST_GCP_PROJECT")
	location := testutil.RequireEnv(t, "LYNX_TEST_GCP_LOCATION")

	modelID, _ := testutil.LookupEnv("LYNX_TEST_VERTEXAI_MODEL")
	if modelID == "" {
		modelID = "gemini-2.0-flash"
	}

	testutil.RunIntegrationChat(t, testutil.IntegrationChatProbe{
		Provider: "vertexai",
		Build: func(t *testing.T, _ string) chat.Model {
			t.Helper()
			opts, err := chat.NewOptions(modelID)
			if err != nil {
				t.Fatal(err)
			}
			m, err := vertexai.NewChatModel(&vertexai.ChatModelConfig{
				Project:        project,
				Location:       location,
				DefaultOptions: opts,
			})
			if err != nil {
				t.Fatal(err)
			}
			return m
		},
	})
}
