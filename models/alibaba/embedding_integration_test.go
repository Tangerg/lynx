//go:build integration

package alibaba_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/modeltest"
	"github.com/Tangerg/lynx/models/alibaba"
)

func TestEmbeddingModel_Integration(t *testing.T) {
	modeltest.RunIntegrationEmbedding(t, modeltest.IntegrationEmbeddingProbe{
		Provider: "alibaba",
		Build: func(t *testing.T, key string) embedding.Model {
			t.Helper()
			modelID, _ := modeltest.LookupEnv("LYNX_TEST_ALIBABA_EMBEDDING_MODEL")
			if modelID == "" {
				modelID = alibaba.ModelEmbeddingV4
			}
			opts, err := embedding.NewOptions(modelID)
			if err != nil {
				t.Fatal(err)
			}
			m, err := alibaba.NewEmbeddingModel(alibaba.EmbeddingModelConfig{
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
