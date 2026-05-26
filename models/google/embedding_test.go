package google_test

import (
	"net/http"
	"testing"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/embedding"
	"github.com/Tangerg/lynx/models/google"
	"github.com/Tangerg/lynx/models/internal/testutil"
)

// genai embed response: { embeddings: [{ values: [...] }, ...] }
const googleEmbedJSON = `{
  "embeddings": [
    {"values": [0.1, 0.2, 0.3]},
    {"values": [0.4, 0.5, 0.6]}
  ]
}`

func TestEmbeddingModel_Call_Mock(t *testing.T) {
	srv := testutil.JSONServer(http.StatusOK, googleEmbedJSON)
	t.Cleanup(srv.Close)

	opts, err := embedding.NewOptions("gemini-embedding-001")
	if err != nil {
		t.Fatal(err)
	}
	m, err := google.NewEmbeddingModel(&google.EmbeddingModelConfig{
		APIKey:         model.NewAPIKey("test-key"),
		DefaultOptions: opts,
		BaseURL:        srv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}

	req, _ := embedding.NewRequest([]string{"foo", "bar"})
	out, err := m.Call(t.Context(), req)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if len(out.Results) != 2 {
		t.Fatalf("got %d results; want 2", len(out.Results))
	}
	if m.Metadata().Provider != google.Provider {
		t.Errorf("provider = %q", m.Metadata().Provider)
	}
}
