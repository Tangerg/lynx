package mistral_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/mistral"
)

const mistralEmbedResponseJSON = `{
  "object": "list",
  "model": "mistral-embed",
  "data": [
    {"object":"embedding","index":0,"embedding":[0.1,0.2,0.3]},
    {"object":"embedding","index":1,"embedding":[0.4,0.5,0.6]}
  ],
  "usage": {"prompt_tokens": 6, "total_tokens": 6}
}`

func TestEmbeddingModel(t *testing.T) {
	testutil.RunEmbeddingContract(t, testutil.EmbeddingContract{
		ProviderName: mistral.Provider,
		ModelID:      mistral.ModelEmbed,
		Response:     mistralEmbedResponseJSON,
		Build: func(t *testing.T, baseURL string) embedding.Model {
			t.Helper()
			opts, err := embedding.NewOptions(mistral.ModelEmbed)
			if err != nil {
				t.Fatalf("NewOptions: %v", err)
			}
			m, err := mistral.NewEmbeddingModel(mistral.EmbeddingModelConfig{
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
