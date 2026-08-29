package vectorstore_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/Tangerg/scope/core/document"
	"github.com/Tangerg/scope/core/vectorstore"
	"github.com/Tangerg/scope/core/vectorstore/filter"
)

func TestModelErrorsAreClassifiable(t *testing.T) {
	if _, err := vectorstore.NewIndexRequest(nil); !errors.Is(err, vectorstore.ErrInvalidRequest) {
		t.Fatalf("NewIndexRequest error = %v, want ErrInvalidRequest", err)
	}
	if _, err := vectorstore.NewSearchRequest(""); !errors.Is(err, vectorstore.ErrInvalidRequest) {
		t.Fatalf("NewSearchRequest error = %v, want ErrInvalidRequest", err)
	}
	if _, err := vectorstore.NewSearchResult(nil, 1); !errors.Is(err, vectorstore.ErrInvalidResponse) {
		t.Fatalf("NewSearchResult error = %v, want ErrInvalidResponse", err)
	}
	if _, err := vectorstore.NewSearchResponse([]*vectorstore.SearchResult{nil}); !errors.Is(err, vectorstore.ErrInvalidResponse) {
		t.Fatalf("NewSearchResponse error = %v, want ErrInvalidResponse", err)
	}
}

func TestJSONRejectsInvalidModels(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  error
	}{
		{name: "index request", value: vectorstore.IndexRequest{}, want: vectorstore.ErrInvalidRequest},
		{name: "search options", value: vectorstore.SearchOptions{TopK: -1}, want: vectorstore.ErrInvalidOptions},
		{name: "search request", value: vectorstore.SearchRequest{}, want: vectorstore.ErrInvalidRequest},
		{name: "search result", value: vectorstore.SearchResult{}, want: vectorstore.ErrInvalidResponse},
		{name: "search response", value: vectorstore.SearchResponse{Results: []*vectorstore.SearchResult{nil}}, want: vectorstore.ErrInvalidResponse},
		{name: "score", value: vectorstore.Score(2), want: vectorstore.ErrInvalidScore},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := json.Marshal(test.value); !errors.Is(err, test.want) {
				t.Fatalf("json.Marshal error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestJSONRoundTripsVectorstoreModels(t *testing.T) {
	doc := &document.Document{ID: "doc-1", Text: "scope"}
	indexRequest, err := vectorstore.NewIndexRequest([]*document.Document{doc})
	if err != nil {
		t.Fatal(err)
	}
	var decodedIndex vectorstore.IndexRequest
	roundTripJSON(t, indexRequest, &decodedIndex)
	if len(decodedIndex.Documents) != 1 || decodedIndex.Documents[0].ID != doc.ID {
		t.Fatalf("decoded index request = %#v", decodedIndex)
	}

	searchRequest := &vectorstore.SearchRequest{
		Query: "cat",
		Options: vectorstore.SearchOptions{
			TopK: 3, MinScore: 0.5, Filter: filter.EQ("kind", "animal"),
		},
	}
	var decodedRequest vectorstore.SearchRequest
	roundTripJSON(t, searchRequest, &decodedRequest)
	if decodedRequest.Query != "cat" || decodedRequest.Options.TopK != 3 || decodedRequest.Options.MinScore != 0.5 {
		t.Fatalf("decoded search request = %#v", decodedRequest)
	}
	if decodedRequest.Options.Filter != nil {
		t.Fatal("wire search request unexpectedly retained the non-wire filter AST")
	}

	response := &vectorstore.SearchResponse{Results: []*vectorstore.SearchResult{{Document: doc, Score: 0.9}}}
	var decodedResponse vectorstore.SearchResponse
	roundTripJSON(t, response, &decodedResponse)
	if len(decodedResponse.Results) != 1 || decodedResponse.Results[0].Document.ID != doc.ID || decodedResponse.Results[0].Score != 0.9 {
		t.Fatalf("decoded search response = %#v", decodedResponse)
	}
}

func TestJSONUnmarshalRejectsInvalidModels(t *testing.T) {
	tests := []struct {
		name   string
		data   string
		target any
		want   error
	}{
		{name: "index request", data: `{"documents":[]}`, target: &vectorstore.IndexRequest{}, want: vectorstore.ErrInvalidRequest},
		{name: "search options", data: `{"top_k":-1}`, target: &vectorstore.SearchOptions{}, want: vectorstore.ErrInvalidOptions},
		{name: "search request", data: `{"query":"","options":{"top_k":1}}`, target: &vectorstore.SearchRequest{}, want: vectorstore.ErrInvalidRequest},
		{name: "search result", data: `{"document":null,"score":1}`, target: &vectorstore.SearchResult{}, want: vectorstore.ErrInvalidResponse},
		{name: "search response", data: `{"results":[null]}`, target: &vectorstore.SearchResponse{}, want: vectorstore.ErrInvalidResponse},
		{name: "score", data: `2`, target: new(vectorstore.Score), want: vectorstore.ErrInvalidScore},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := json.Unmarshal([]byte(test.data), test.target); !errors.Is(err, test.want) {
				t.Fatalf("json.Unmarshal error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestNilJSONReceiversAreRejected(t *testing.T) {
	var indexRequest *vectorstore.IndexRequest
	if err := indexRequest.UnmarshalJSON([]byte(`{}`)); !errors.Is(err, vectorstore.ErrInvalidRequest) {
		t.Fatalf("IndexRequest.UnmarshalJSON error = %v", err)
	}
	var options *vectorstore.SearchOptions
	if err := options.UnmarshalJSON([]byte(`{}`)); !errors.Is(err, vectorstore.ErrInvalidOptions) {
		t.Fatalf("SearchOptions.UnmarshalJSON error = %v", err)
	}
	var request *vectorstore.SearchRequest
	if err := request.UnmarshalJSON([]byte(`{}`)); !errors.Is(err, vectorstore.ErrInvalidRequest) {
		t.Fatalf("SearchRequest.UnmarshalJSON error = %v", err)
	}
	var result *vectorstore.SearchResult
	if err := result.UnmarshalJSON([]byte(`{}`)); !errors.Is(err, vectorstore.ErrInvalidResponse) {
		t.Fatalf("SearchResult.UnmarshalJSON error = %v", err)
	}
	var response *vectorstore.SearchResponse
	if err := response.UnmarshalJSON([]byte(`{}`)); !errors.Is(err, vectorstore.ErrInvalidResponse) {
		t.Fatalf("SearchResponse.UnmarshalJSON error = %v", err)
	}
	var score *vectorstore.Score
	if err := score.UnmarshalJSON([]byte(`0`)); !errors.Is(err, vectorstore.ErrInvalidScore) {
		t.Fatalf("Score.UnmarshalJSON error = %v", err)
	}
}

func roundTripJSON(t *testing.T, source, target any) {
	t.Helper()
	data, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatal(err)
	}
}
