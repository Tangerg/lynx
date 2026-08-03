//go:build integration

package vertexai_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/modeltest"
	"github.com/Tangerg/lynx/models/google/vertexai"
)

func TestEmbeddingModel_Integration(t *testing.T) {
	modeltest.RequireKey(t, "vertexai")
	project := modeltest.RequireEnv(t, "LYNX_TEST_GCP_PROJECT")
	location := modeltest.RequireEnv(t, "LYNX_TEST_GCP_LOCATION")

	modelID, _ := modeltest.LookupEnv("LYNX_TEST_VERTEXAI_EMBEDDING_MODEL")
	if modelID == "" {
		modelID = "gemini-embedding-001"
	}

	modeltest.RunIntegrationEmbedding(t, modeltest.IntegrationEmbeddingProbe{
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
