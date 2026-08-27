//go:build integration

package nomic_test

import (
	"testing"

	"github.com/Tangerg/scope/core/embedding"
	"github.com/Tangerg/scope/core/modeltest"
	"github.com/Tangerg/scope/models/nomic"
)

func TestEmbeddingModel_Integration(t *testing.T) {
	modeltest.RunIntegrationEmbedding(t, modeltest.IntegrationEmbeddingProbe{
		Provider: "nomic",
		Build: func(t *testing.T, key string) embedding.Model {
			t.Helper()
			modelID, _ := modeltest.LookupEnv("SCOPE_TEST_NOMIC_MODEL")
			if modelID == "" {
				modelID = nomic.ModelEmbedTextV15
			}
			opts, err := embedding.NewOptions(modelID)
			if err != nil {
				t.Fatal(err)
			}
			m, err := nomic.NewEmbeddingModel(nomic.EmbeddingModelConfig{
				APIKey:         key,
				DefaultOptions: opts,
			})
			if err != nil {
				t.Fatal(err)
			}
			return m
		},
	})
}
