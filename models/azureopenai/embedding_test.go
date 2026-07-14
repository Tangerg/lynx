package azureopenai_test

import (
	"net/http"
	"testing"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/models/azureopenai"
	"github.com/Tangerg/lynx/models/internal/testutil"
)

const azureEmbedJSON = `{
  "object": "list",
  "model": "text-embedding-ada-002",
  "data": [
    {"object":"embedding","index":0,"embedding":[0.1,0.2,0.3]},
    {"object":"embedding","index":1,"embedding":[0.4,0.5,0.6]}
  ],
  "usage": {"prompt_tokens": 6, "total_tokens": 6}
}`

func TestEmbeddingModel_Call_Mock(t *testing.T) {
	srv := testutil.JSONServer(http.StatusOK, azureEmbedJSON)
	t.Cleanup(srv.Close)

	opts, err := embedding.NewOptions("text-embedding-ada-002")
	if err != nil {
		t.Fatal(err)
	}
	m, err := azureopenai.NewEmbeddingModel(azureopenai.EmbeddingModelConfig{
		APIKey:         "test-key",
		Endpoint:       srv.URL,
		DefaultOptions: opts,
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
