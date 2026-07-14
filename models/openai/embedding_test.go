package openai_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/models/internal/testutil"
	"github.com/Tangerg/lynx/models/openai"
)

func newEmbeddingModel(t *testing.T, baseURL, modelID string) *openai.EmbeddingModel {
	t.Helper()
	opts, err := embedding.NewOptions(modelID)
	if err != nil {
		t.Fatalf("NewOptions: %v", err)
	}
	m, err := openai.NewEmbeddingModel(openai.EmbeddingModelConfig{
		APIKey:         model.NewAPIKey("test-key"),
		DefaultOptions: opts,
		RequestOptions: []option.RequestOption{option.WithBaseURL(baseURL)},
	})
	if err != nil {
		t.Fatalf("NewEmbeddingModel: %v", err)
	}
	return m
}

func TestEmbeddingModel_Call_Mock(t *testing.T) {
	resp := openaisdk.CreateEmbeddingResponse{
		Object: "list",
		Model:  "text-embedding-3-small",
		Data: []openaisdk.Embedding{
			{Object: "embedding", Index: 0, Embedding: []float64{0.1, 0.2, 0.3}},
			{Object: "embedding", Index: 1, Embedding: []float64{0.4, 0.5, 0.6}},
		},
		Usage: openaisdk.CreateEmbeddingResponseUsage{
			PromptTokens: 8,
			TotalTokens:  8,
		},
	}
	body, _ := json.Marshal(resp)

	var seenURL string
	srv := testutil.JSONServer(http.StatusOK, string(body), func(r *http.Request) {
		seenURL = r.URL.Path
	})
	t.Cleanup(srv.Close)

	m := newEmbeddingModel(t, srv.URL, "text-embedding-3-small")
	req, err := embedding.NewRequest([]string{"hello", "world"})
	if err != nil {
		t.Fatal(err)
	}

	out, err := m.Call(t.Context(), req)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !strings.HasSuffix(seenURL, "/embeddings") {
		t.Errorf("URL = %q; want suffix /embeddings", seenURL)
	}
	if len(out.Results) != 2 {
		t.Fatalf("got %d results; want 2", len(out.Results))
	}
	if out.Metadata.Usage == nil || out.Metadata.Usage.PromptTokens != 8 {
		t.Errorf("usage = %+v; want PromptTokens=8", out.Metadata.Usage)
	}
}

func TestEmbeddingModel_Metadata(t *testing.T) {
	srv := testutil.JSONServer(http.StatusOK, "{}")
	t.Cleanup(srv.Close)
	m := newEmbeddingModel(t, srv.URL, "text-embedding-3-small")
	if m.Metadata().Provider != openai.Provider {
		t.Errorf("provider = %q; want %q", m.Metadata().Provider, openai.Provider)
	}
}
