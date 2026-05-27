package voyage_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/embedding"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/voyage"
)

const voyageResponseJSON = `{
  "object": "list",
  "data": [
    {"object":"embedding","embedding":[0.1,0.2,0.3],"index":0},
    {"object":"embedding","embedding":[0.4,0.5,0.6],"index":1}
  ],
  "model": "voyage-3-large",
  "usage": {"total_tokens": 6}
}`

func TestEmbeddingModel(t *testing.T) {
	testutil.RunEmbeddingContract(t, testutil.EmbeddingContract{
		ProviderName: voyage.Provider,
		ModelID:      "voyage-3-large",
		Response:     voyageResponseJSON,
		ExpectedPath: "/embeddings",
		Build: func(t *testing.T, baseURL string) embedding.Model {
			t.Helper()
			opts, err := embedding.NewOptions("voyage-3-large")
			if err != nil {
				t.Fatalf("NewOptions: %v", err)
			}
			m, err := voyage.NewEmbeddingModel(voyage.EmbeddingModelConfig{
				APIKey:         model.NewAPIKey("test-key"),
				DefaultOptions: opts,
				BaseURL:        baseURL,
			})
			if err != nil {
				t.Fatalf("NewEmbeddingModel: %v", err)
			}
			return m
		},
	})
}
