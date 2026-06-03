//go:build integration

package cohere_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/embedding"
	"github.com/Tangerg/lynx/models/cohere"
	"github.com/Tangerg/lynx/models/internal/testutil"
)

func TestEmbeddingModel_Integration(t *testing.T) {
	testutil.RunIntegrationEmbedding(t, testutil.IntegrationEmbeddingProbe{
		Provider: "cohere",
		Build: func(t *testing.T, key string) embedding.Model {
			t.Helper()
			modelID, _ := testutil.LookupEnv("LYNX_TEST_COHERE_MODEL")
			if modelID == "" {
				modelID = "embed-english-v3.0"
			}
			opts, err := embedding.NewOptions(modelID)
			if err != nil {
				t.Fatal(err)
			}
			m, err := cohere.NewEmbeddingModel(cohere.EmbeddingModelConfig{
				APIKey:         model.NewAPIKey(key),
				DefaultOptions: opts,
			})
			if err != nil {
				t.Fatal(err)
			}
			return m
		},
	})
}
