package azureaisearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tangerg/scope/core/embedding"
	"github.com/Tangerg/scope/core/embeddingclient"
	"github.com/Tangerg/scope/core/vectorstore"
)

func TestHybridSearchSendsLexicalAndVectorEvidence(t *testing.T) {
	t.Parallel()

	seen := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		seen <- body
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"value":[{"id":"one","content":"Paris","@search.score":0.03}]}`))
	}))
	t.Cleanup(server.Close)

	embeddingClient, err := embeddingclient.New(embedding.ModelFunc(func(context.Context, *embedding.Request) (*embedding.Response, error) {
		output, err := embedding.NewOutput([]float64{1, 0}, nil)
		if err != nil {
			return nil, err
		}
		return embedding.NewResponse([]*embedding.Output{output}, nil)
	}))
	if err != nil {
		t.Fatal(err)
	}
	store := &Store{
		endpoint: server.URL, apiKey: "key", indexName: "documents", apiVersion: DefaultAPIVersion,
		idField: DefaultIDField, contentField: DefaultContentField, embeddingField: DefaultEmbeddingField,
		embeddingClient: embeddingClient, similarityMetric: SimilarityCosine, httpClient: server.Client(),
	}
	response, err := store.Search(t.Context(), &vectorstore.SearchRequest{
		Query: "capital of France", Options: vectorstore.SearchOptions{Mode: vectorstore.SearchModeHybrid, TopK: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Results) != 1 || response.Results[0].Document.ID != "one" {
		t.Fatalf("Search() = %#v", response)
	}

	body := <-seen
	if body["search"] != "capital of France" || body["searchFields"] != DefaultContentField {
		t.Fatalf("hybrid lexical request = %#v", body)
	}
	if vectorQueries, ok := body["vectorQueries"].([]any); !ok || len(vectorQueries) != 1 {
		t.Fatalf("hybrid vector request = %#v", body["vectorQueries"])
	}
}
