package modeltest

import (
	"net/http"
	"testing"
	"time"

	"github.com/Tangerg/scope/core/embedding"
)

const integrationEmbeddingTimeout = 30 * time.Second

// EmbeddingContract drives the mock-test contract for any embedding
// vendor. The `Response` field is the canned JSON body the mock server
// returns — it should encode a response with 2 embeddings (matching
// the 2-input request the contract sends).
type EmbeddingContract struct {
	// ModelID is the model id passed into the embedding request.
	ModelID string
	// Response is the canned JSON body — must encode 2 outputs so the
	// contract can validate batching.
	Response string
	// ExpectedPath is the URL path the SDK should hit (e.g. "/embeddings"
	// or "/embedding/text"). Empty means skip the path assertion.
	ExpectedPath string
	// Build returns the model wired against the mock server.
	Build func(t *testing.T, baseURL string) embedding.Model
}

func RunEmbeddingContract(t *testing.T, contract EmbeddingContract) {
	t.Helper()
	t.Run("Call_Mock", func(t *testing.T) {
		var seenPath string
		server := JSONServer(http.StatusOK, contract.Response, func(request *http.Request) {
			seenPath = request.URL.Path
		})
		t.Cleanup(server.Close)

		model := contract.Build(t, server.URL)
		request, err := embedding.NewRequest([]string{"foo", "bar"})
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}

		response, err := model.Call(t.Context(), request)
		if err != nil {
			t.Fatalf("Call: %v", err)
		}
		if contract.ExpectedPath != "" && seenPath != contract.ExpectedPath {
			t.Errorf("URL = %q; want %q", seenPath, contract.ExpectedPath)
		}
		if len(response.Outputs) != len(request.Texts) {
			t.Fatalf("got %d outputs; want %d", len(response.Outputs), len(request.Texts))
		}
		for index, output := range response.Outputs {
			if len(output.Embedding) == 0 {
				t.Errorf("output %d has empty embedding", index)
			}
		}
	})

}

// IntegrationEmbeddingProbe is the standard real-API embedding smoke
// probe: Call returns 2 outputs with non-empty embeddings.
type IntegrationEmbeddingProbe struct {
	Provider string
	Build    func(t *testing.T, key string) embedding.Model
}

func RunIntegrationEmbedding(t *testing.T, probe IntegrationEmbeddingProbe) {
	t.Helper()
	key := RequireKey(t, probe.Provider)
	model := probe.Build(t, key)
	ctx, cancel := WithTimeout(t, integrationEmbeddingTimeout)
	defer cancel()

	request, err := embedding.NewRequest([]string{"the quick brown fox", "jumps over the lazy dog"})
	if err != nil {
		t.Fatal(err)
	}
	response, err := model.Call(ctx, request)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if len(response.Outputs) != len(request.Texts) {
		t.Fatalf("got %d outputs; want %d", len(response.Outputs), len(request.Texts))
	}
	for index, output := range response.Outputs {
		if len(output.Embedding) == 0 {
			t.Errorf("output %d has empty embedding", index)
		}
	}
}
