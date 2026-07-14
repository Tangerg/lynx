//go:build integration

package openai_test

import (
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/openai"
)

func integrationEmbeddingModel(t *testing.T) *openai.EmbeddingModel {
	t.Helper()
	key := testutil.RequireKey(t, "openai")
	modelID, _ := testutil.LookupEnv("LYNX_TEST_OPENAI_EMBEDDING_MODEL")
	if modelID == "" {
		modelID = "text-embedding-3-small"
	}
	opts, err := embedding.NewOptions(modelID)
	if err != nil {
		t.Fatal(err)
	}
	m, err := openai.NewEmbeddingModel(openai.EmbeddingModelConfig{
		APIKey:         model.NewAPIKey(key),
		DefaultOptions: opts,
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestEmbeddingModel_Call_Integration(t *testing.T) {
	m := integrationEmbeddingModel(t)
	req, err := embedding.NewRequest([]string{"the quick brown fox", "jumps over the lazy dog"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := testutil.WithTimeout(t, 30*time.Second)
	defer cancel()

	resp, err := m.Call(ctx, req)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("got %d results; want 2", len(resp.Results))
	}
	for i, r := range resp.Results {
		if len(r.Embedding) == 0 {
			t.Errorf("result %d has empty embedding", i)
		}
	}
}
