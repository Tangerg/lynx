//go:build integration

package cohere_test

import (
	"testing"

	coheresdk "github.com/cohere-ai/cohere-go/v2"

	"github.com/Tangerg/scope/core/embedding"
	"github.com/Tangerg/scope/models/cohere"
)

func TestEmbeddingModel_Integration(t *testing.T) {
	runIntegrationEmbedding(t, integrationEmbeddingProbe{
		Provider: "cohere",
		Build: func(t *testing.T, key string) embedding.Model {
			t.Helper()
			modelID, _ := lookupEnv("SCOPE_TEST_COHERE_MODEL")
			if modelID == "" {
				modelID = "embed-v4.0"
			}
			opts, err := embedding.NewOptions(modelID)
			if err != nil {
				t.Fatal(err)
			}
			if err := opts.SetExtension(cohere.EmbeddingRequestExtensionKey, coheresdk.V2EmbedRequest{
				InputType: coheresdk.EmbedInputTypeSearchDocument,
			}); err != nil {
				t.Fatal(err)
			}
			m, err := cohere.NewEmbeddingModel(cohere.EmbeddingModelConfig{
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
