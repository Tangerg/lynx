//go:build integration

package cohere_test

import (
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/embedding"
)

// integrationEmbeddingProbe is the standard real-API embedding smoke
// probe: Call returns 2 results with non-empty embeddings.
type integrationEmbeddingProbe struct {
	Provider string
	Build    func(t *testing.T, key string) embedding.Model
}

func runIntegrationEmbedding(t *testing.T, p integrationEmbeddingProbe) {
	t.Helper()
	key := requireKey(t, p.Provider)
	m := p.Build(t, key)
	ctx, cancel := withTimeout(t, 30*time.Second)
	defer cancel()

	req, err := embedding.NewRequest([]string{"the quick brown fox", "jumps over the lazy dog"})
	if err != nil {
		t.Fatal(err)
	}
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
