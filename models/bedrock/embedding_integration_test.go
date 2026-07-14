//go:build integration

package bedrock_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/models/bedrock"
	"github.com/Tangerg/lynx/models/internal/testutil"
)

func TestEmbeddingModel_Integration(t *testing.T) {
	testutil.RequireKey(t, "bedrock")
	region := testutil.RequireEnv(t, "AWS_REGION")

	modelID, _ := testutil.LookupEnv("LYNX_TEST_BEDROCK_EMBEDDING_MODEL")
	if modelID == "" {
		modelID = "amazon.titan-embed-text-v2:0"
	}

	testutil.RunIntegrationEmbedding(t, testutil.IntegrationEmbeddingProbe{
		Provider: "bedrock",
		Build: func(t *testing.T, _ string) embedding.Model {
			t.Helper()
			opts, err := embedding.NewOptions(modelID)
			if err != nil {
				t.Fatal(err)
			}
			m, err := bedrock.NewEmbeddingModel(t.Context(), bedrock.EmbeddingModelConfig{
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
