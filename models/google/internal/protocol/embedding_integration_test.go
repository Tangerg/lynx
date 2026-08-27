//go:build integration

package protocol_test

import (
	"testing"

	"github.com/Tangerg/scope/core/embedding"
	"github.com/Tangerg/scope/core/modeltest"
	"github.com/Tangerg/scope/models/google/internal/protocol"
)

func TestEmbeddingModel_Integration(t *testing.T) {
	modeltest.RunIntegrationEmbedding(t, modeltest.IntegrationEmbeddingProbe{
		Provider: "google",
		Build: func(t *testing.T, key string) embedding.Model {
			t.Helper()
			modelID, _ := modeltest.LookupEnv("SCOPE_TEST_GOOGLE_EMBEDDING_MODEL")
			if modelID == "" {
				modelID = protocol.ModelGeminiEmbedding2
			}
			opts, err := embedding.NewOptions(modelID)
			if err != nil {
				t.Fatal(err)
			}
			m, err := protocol.NewEmbeddingModel(protocol.EmbeddingModelConfig{
				Provider:       "google",
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
