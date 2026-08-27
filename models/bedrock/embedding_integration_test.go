//go:build integration

package bedrock_test

import (
	"testing"

	"github.com/Tangerg/scope/core/embedding"
	"github.com/Tangerg/scope/models/bedrock"
)

func TestEmbeddingModel_Integration(t *testing.T) {
	requireKey(t, "bedrock")
	region := requireEnv(t, "AWS_REGION")

	modelID, _ := lookupEnv("SCOPE_TEST_BEDROCK_EMBEDDING_MODEL")
	if modelID == "" {
		modelID = "amazon.titan-embed-text-v2:0"
	}

	runIntegrationEmbedding(t, integrationEmbeddingProbe{
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
