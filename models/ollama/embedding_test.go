package ollama_test

import (
	"net/http"
	"testing"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/models/ollama"
)

const ollamaEmbedJSON = `{
  "model": "nomic-embed-text",
  "embeddings": [[0.1, 0.2, 0.3], [0.4, 0.5, 0.6]],
  "total_duration": 1000000,
  "load_duration": 100000,
  "prompt_eval_count": 6
}`

func TestEmbeddingModel_Call_Mock(t *testing.T) {
	srv := jsonServer(http.StatusOK, ollamaEmbedJSON, func(r *http.Request) {
		if r.URL.Path != "/api/embed" {
			t.Errorf("path = %q, want /api/embed", r.URL.Path)
		}
		if accept := r.Header.Get("Accept"); accept != "application/json" {
			t.Errorf("Accept = %q, want application/json", accept)
		}
		if authorization := r.Header.Get("Authorization"); authorization != "" {
			t.Errorf("unexpected implicit credential %q", authorization)
		}
	})
	t.Cleanup(srv.Close)

	opts, err := embedding.NewOptions("nomic-embed-text")
	if err != nil {
		t.Fatal(err)
	}
	m, err := ollama.NewEmbeddingModel(ollama.EmbeddingModelConfig{
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
}
