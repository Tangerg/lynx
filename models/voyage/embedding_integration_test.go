//go:build integration

package voyage_test

import (
	"testing"

	"github.com/Tangerg/scope/core/embedding"
	"github.com/Tangerg/scope/core/modeltest"
	"github.com/Tangerg/scope/models/voyage"
)

func TestEmbeddingModel_Integration(t *testing.T) {
	modeltest.RunIntegrationEmbedding(t, modeltest.IntegrationEmbeddingProbe{
		Provider: "voyage",
		Build: func(t *testing.T, key string) embedding.Model {
			t.Helper()
			modelID, _ := modeltest.LookupEnv("SCOPE_TEST_VOYAGE_MODEL")
			if modelID == "" {
				modelID = "voyage-3-lite"
			}
			opts, err := embedding.NewOptions(modelID)
			if err != nil {
				t.Fatal(err)
			}
			m, err := voyage.NewEmbeddingModel(voyage.EmbeddingModelConfig{
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
