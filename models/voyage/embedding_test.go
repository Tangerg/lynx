package voyage_test

import (
	"net/http"
	"testing"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/embedding"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/voyage"
)

func newEmbeddingModel(t *testing.T, baseURL, modelID string) *voyage.EmbeddingModel {
	t.Helper()
	opts, err := embedding.NewOptions(modelID)
	if err != nil {
		t.Fatalf("NewOptions: %v", err)
	}
	m, err := voyage.NewEmbeddingModel(&voyage.EmbeddingModelConfig{
		ApiKey:         model.NewApiKey("test-key"),
		DefaultOptions: opts,
		BaseURL:        baseURL,
	})
	if err != nil {
		t.Fatalf("NewEmbeddingModel: %v", err)
	}
	return m
}

const voyageResponseJSON = `{
  "object": "list",
  "data": [
    {"object":"embedding","embedding":[0.1,0.2,0.3],"index":0},
    {"object":"embedding","embedding":[0.4,0.5,0.6],"index":1}
  ],
  "model": "voyage-3-large",
  "usage": {"total_tokens": 6}
}`

func TestEmbeddingModel_Call_Mock(t *testing.T) {
	var seenAuth, seenURL string
	srv := testutil.JSONServer(http.StatusOK, voyageResponseJSON, func(r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		seenURL = r.URL.Path
	})
	t.Cleanup(srv.Close)

	m := newEmbeddingModel(t, srv.URL, "voyage-3-large")
	req, _ := embedding.NewRequest([]string{"foo", "bar"})

	out, err := m.Call(t.Context(), req)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if seenAuth != "Bearer test-key" {
		t.Errorf("Authorization = %q; want Bearer test-key", seenAuth)
	}
	if seenURL != "/embeddings" {
		t.Errorf("URL = %q; want /embeddings", seenURL)
	}
	if len(out.Results) != 2 {
		t.Fatalf("got %d results; want 2", len(out.Results))
	}
	if out.Metadata.Usage == nil || out.Metadata.Usage.PromptTokens != 6 {
		t.Errorf("usage = %+v; want PromptTokens=6", out.Metadata.Usage)
	}
}

func TestEmbeddingModel_Metadata(t *testing.T) {
	srv := testutil.JSONServer(http.StatusOK, "{}")
	t.Cleanup(srv.Close)
	m := newEmbeddingModel(t, srv.URL, "voyage-3-large")
	if m.Metadata().Provider != voyage.Provider {
		t.Errorf("provider = %q; want %q", m.Metadata().Provider, voyage.Provider)
	}
}
