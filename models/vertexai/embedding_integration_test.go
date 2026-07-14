//go:build integration

package vertexai_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/vertexai"
)

func TestEmbeddingModel_Integration(t *testing.T) {
	testutil.RequireKey(t, "vertexai")
	project := testutil.RequireEnv(t, "LYNX_TEST_GCP_PROJECT")
	location := testutil.RequireEnv(t, "LYNX_TEST_GCP_LOCATION")

	modelID, _ := testutil.LookupEnv("LYNX_TEST_VERTEXAI_EMBEDDING_MODEL")
	if modelID == "" {
		modelID = "gemini-embedding-001"
	}

	testutil.RunIntegrationEmbedding(t, testutil.IntegrationEmbeddingProbe{
		Provider: "vertexai",
		Build: func(t *testing.T, _ string) embedding.Model {
			t.Helper()
			opts, err := embedding.NewOptions(modelID)
			if err != nil {
				t.Fatal(err)
			}
			m, err := vertexai.NewEmbeddingModel(vertexai.EmbeddingModelConfig{
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
