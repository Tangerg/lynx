//go:build integration

package nomic_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/nomic"
)

func TestEmbeddingModel_Integration(t *testing.T) {
	testutil.RunIntegrationEmbedding(t, testutil.IntegrationEmbeddingProbe{
		Provider: "nomic",
		Build: func(t *testing.T, key string) embedding.Model {
			t.Helper()
			modelID, _ := testutil.LookupEnv("LYNX_TEST_NOMIC_MODEL")
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
