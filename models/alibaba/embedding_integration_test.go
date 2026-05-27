//go:build integration

package alibaba_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/embedding"
	"github.com/Tangerg/lynx/models/alibaba"
	"github.com/Tangerg/lynx/models/internal/testutil"
)

func TestEmbeddingModel_Integration(t *testing.T) {
	testutil.RunIntegrationEmbedding(t, testutil.IntegrationEmbeddingProbe{
		Provider: "alibaba",
		Build: func(t *testing.T, key string) embedding.Model {
			t.Helper()
			modelID, _ := testutil.LookupEnv("LYNX_TEST_ALIBABA_EMBEDDING_MODEL")
			if modelID == "" {
				modelID = alibaba.ModelEmbeddingV4
			}
			opts, err := embedding.NewOptions(modelID)
			if err != nil {
				t.Fatal(err)
			}
			m, err := alibaba.NewEmbeddingModel(alibaba.EmbeddingModelConfig{
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
