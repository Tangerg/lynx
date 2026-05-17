//go:build integration

package ollama_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/model/embedding"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/ollama"
)

func TestEmbeddingModel_Integration(t *testing.T) {
	testutil.RunIntegrationEmbedding(t, testutil.IntegrationEmbeddingProbe{
		Provider: "ollama",
		Build: func(t *testing.T, _ string) embedding.Model {
			t.Helper()
			modelID, _ := testutil.LookupEnv("LYNX_TEST_OLLAMA_EMBEDDING_MODEL")
			if modelID == "" {
				modelID = "nomic-embed-text"
			}
			baseURL, _ := testutil.LookupEnv("LYNX_TEST_OLLAMA_BASE_URL")
			opts, err := embedding.NewOptions(modelID)
			if err != nil {
				t.Fatal(err)
			}
			m, err := ollama.NewEmbeddingModel(&ollama.EmbeddingModelConfig{
				DefaultOptions: opts,
				BaseURL:        baseURL,
			})
			if err != nil {
				t.Fatal(err)
			}
			return m
		},
	})
}
