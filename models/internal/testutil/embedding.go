package testutil

import (
	"net/http"
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/embedding"
)

// EmbeddingContract drives the mock-test contract for any embedding
// vendor. The `Response` field is the canned JSON body the mock server
// returns — it should encode a response with 2 embeddings (matching
// the 2-input request the contract sends).
type EmbeddingContract struct {
	// ProviderName is asserted against Model.Metadata().Provider.
	ProviderName string
	// ModelID is the model id passed into the embedding request.
	ModelID string
	// Response is the canned JSON body — must encode 2 results so the
	// contract can validate batching.
	Response string
	// ExpectedPath is the URL path the SDK should hit (e.g. "/embeddings"
	// or "/embedding/text"). Empty means skip the path assertion.
	ExpectedPath string
	// Build returns the model wired against the mock server.
	Build func(t *testing.T, baseURL string) embedding.Model
}

// RunEmbeddingContract exercises an embedding vendor against canned
// JSON: Call returns 2 results with non-empty embeddings, and the
// model's Metadata reports the expected provider.
func RunEmbeddingContract(t *testing.T, c EmbeddingContract) {
	t.Helper()
	t.Run("Call_Mock", func(t *testing.T) {
		var seenPath string
		srv := JSONServer(http.StatusOK, c.Response, func(r *http.Request) {
			seenPath = r.URL.Path
		})
		t.Cleanup(srv.Close)

		m := c.Build(t, srv.URL)
		req, err := embedding.NewRequest([]string{"foo", "bar"})
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}

		resp, err := m.Call(t.Context(), req)
		if err != nil {
			t.Fatalf("Call: %v", err)
		}
		if c.ExpectedPath != "" && seenPath != c.ExpectedPath {
			t.Errorf("URL = %q; want %q", seenPath, c.ExpectedPath)
		}
		if len(resp.Results) != 2 {
			t.Fatalf("got %d results; want 2", len(resp.Results))
		}
		for i, r := range resp.Results {
			if len(r.Embedding) == 0 {
				t.Errorf("result %d has empty embedding", i)
			}
		}
	})

	t.Run("Metadata", func(t *testing.T) {
		srv := JSONServer(http.StatusOK, "{}")
		t.Cleanup(srv.Close)
		m := c.Build(t, srv.URL)
		if got := m.Metadata().Provider; got != c.ProviderName {
			t.Errorf("provider = %q; want %q", got, c.ProviderName)
		}
	})
}

// IntegrationEmbeddingProbe is the standard real-API embedding smoke
// probe: Call returns 2 results with non-empty embeddings.
type IntegrationEmbeddingProbe struct {
	Provider string
	Build    func(t *testing.T, key string) embedding.Model
}

func RunIntegrationEmbedding(t *testing.T, p IntegrationEmbeddingProbe) {
	t.Helper()
	key := RequireKey(t, p.Provider)
	m := p.Build(t, key)
	ctx, cancel := WithTimeout(t, 30*time.Second)
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
