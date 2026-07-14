//go:build integration

package jina_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/jina"
)

func TestEmbeddingModel_Integration(t *testing.T) {
	testutil.RunIntegrationEmbedding(t, testutil.IntegrationEmbeddingProbe{
		Provider: "jina",
		Build: func(t *testing.T, key string) embedding.Model {
			t.Helper()
			modelID, _ := testutil.LookupEnv("LYNX_TEST_JINA_MODEL")
			if modelID == "" {
				modelID = jina.ModelEmbeddingsV3
			}
			opts, err := embedding.NewOptions(modelID)
			if err != nil {
				t.Fatal(err)
			}
			m, err := jina.NewEmbeddingModel(jina.EmbeddingModelConfig{
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
