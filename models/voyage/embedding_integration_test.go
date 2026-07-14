//go:build integration

package voyage_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/voyage"
)

func TestEmbeddingModel_Integration(t *testing.T) {
	testutil.RunIntegrationEmbedding(t, testutil.IntegrationEmbeddingProbe{
		Provider: "voyage",
		Build: func(t *testing.T, key string) embedding.Model {
			t.Helper()
			modelID, _ := testutil.LookupEnv("LYNX_TEST_VOYAGE_MODEL")
			if modelID == "" {
				modelID = "voyage-3-lite"
			}
			opts, err := embedding.NewOptions(modelID)
			if err != nil {
				t.Fatal(err)
			}
			m, err := voyage.NewEmbeddingModel(voyage.EmbeddingModelConfig{
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
