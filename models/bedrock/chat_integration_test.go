//go:build integration

package bedrock_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/bedrock"
	"github.com/Tangerg/lynx/models/internal/testutil"
)

// Bedrock authenticates via the standard AWS credential chain — set
// AWS_REGION + the usual AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY (or
// AWS profile / IRSA / IAM role) plus LYNX_TEST_BEDROCK_KEY as the
// sentinel that enables this test.
func TestChatModel_Integration(t *testing.T) {
	testutil.RequireKey(t, "bedrock")
	region := testutil.RequireEnv(t, "AWS_REGION")

	modelID, _ := testutil.LookupEnv("LYNX_TEST_BEDROCK_MODEL")
	if modelID == "" {
		modelID = "anthropic.claude-3-5-haiku-20241022-v1:0"
	}

	testutil.RunIntegrationChat(t, testutil.IntegrationChatProbe{
		Provider: "bedrock",
		Build: func(t *testing.T, _ string) chat.Model {
			t.Helper()
			opts, err := chat.NewOptions(modelID)
			if err != nil {
				t.Fatal(err)
			}
			m, err := bedrock.NewChatModel(t.Context(), bedrock.ChatModelConfig{
				DefaultOptions: opts,
				Region:         region,
			})
			if err != nil {
				t.Fatal(err)
			}
			return m
		},
	})
}
