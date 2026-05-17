//go:build integration

package voyage_test

import (
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/embedding"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/voyage"
)

func integrationModel(t *testing.T) *voyage.EmbeddingModel {
	t.Helper()
	key := testutil.RequireKey(t, "voyage")
	modelID, _ := testutil.LookupEnv("LYNX_TEST_VOYAGE_MODEL")
	if modelID == "" {
		modelID = "voyage-3-lite"
	}
	opts, err := embedding.NewOptions(modelID)
	if err != nil {
		t.Fatal(err)
	}
	m, err := voyage.NewEmbeddingModel(&voyage.EmbeddingModelConfig{
		ApiKey:         model.NewApiKey(key),
		DefaultOptions: opts,
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestEmbeddingModel_Call_Integration(t *testing.T) {
	m := integrationModel(t)
	req, _ := embedding.NewRequest([]string{"the quick brown fox", "jumps over the lazy dog"})
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
