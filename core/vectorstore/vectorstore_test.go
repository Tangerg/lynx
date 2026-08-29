package vectorstore_test

import (
	"math"
	"reflect"
	"testing"

	"github.com/Tangerg/scope/core/document"
	"github.com/Tangerg/scope/core/vectorstore"
)

func TestSearchRequestValidate(t *testing.T) {
	cases := []struct {
		name string
		req  vectorstore.SearchRequest
		ok   bool
	}{
		{"empty query", vectorstore.SearchRequest{Options: vectorstore.SearchOptions{TopK: 5}}, false},
		{"default topk", vectorstore.SearchRequest{Query: "hi"}, true},
		{"negative topk", vectorstore.SearchRequest{Query: "hi", Options: vectorstore.SearchOptions{TopK: -1}}, false},
		{"out-of-range minscore", vectorstore.SearchRequest{Query: "hi", Options: vectorstore.SearchOptions{TopK: 5, MinScore: 1.5}}, false},
		{"nan minscore", vectorstore.SearchRequest{Query: "hi", Options: vectorstore.SearchOptions{TopK: 5, MinScore: vectorstore.Score(math.NaN())}}, false},
		{"valid", vectorstore.SearchRequest{Query: "hi", Options: vectorstore.SearchOptions{TopK: 5, MinScore: 0.5}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if tc.ok && err != nil {
				t.Fatalf("expected ok, got %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestSearchOptionsZeroValueUsesDefaultLimit(t *testing.T) {
	var options vectorstore.SearchOptions
	if err := options.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := options.ResultLimit(); got != vectorstore.DefaultTopK {
		t.Fatalf("ResultLimit = %d, want %d", got, vectorstore.DefaultTopK)
	}
	options.TopK = 3
	if got := options.ResultLimit(); got != 3 {
		t.Fatalf("explicit ResultLimit = %d, want 3", got)
	}
}

func TestSearchResponseValidate(t *testing.T) {
	request := vectorstore.SearchRequest{Query: "scope", Options: vectorstore.SearchOptions{TopK: 2, MinScore: 0.5}}
	first, _ := document.NewDocument("first", nil)
	second, _ := document.NewDocument("second", nil)
	first.ID = "first"
	second.ID = "second"
	valid := []*vectorstore.SearchResult{{Document: first, Score: 0.9}, {Document: second, Score: 0.5}}
	if err := (&vectorstore.SearchResponse{Results: valid}).ValidateFor(&request); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		results []*vectorstore.SearchResult
	}{
		{name: "too many", results: append(valid, &vectorstore.SearchResult{Document: second, Score: 0.5})},
		{name: "nil result", results: []*vectorstore.SearchResult{nil}},
		{name: "nil document", results: []*vectorstore.SearchResult{{Score: 0.9}}},
		{name: "missing document ID", results: []*vectorstore.SearchResult{{Document: &document.Document{Text: "text"}, Score: 0.9}}},
		{name: "out of range", results: []*vectorstore.SearchResult{{Document: first, Score: 1.1}}},
		{name: "below threshold", results: []*vectorstore.SearchResult{{Document: first, Score: 0.4}}},
		{name: "not sorted", results: []*vectorstore.SearchResult{{Document: first, Score: 0.5}, {Document: second, Score: 0.9}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := (&vectorstore.SearchResponse{Results: test.results}).ValidateFor(&request); err == nil {
				t.Fatal("SearchResponse.Validate accepted invalid output")
			}
		})
	}
}

func TestCapabilityInterfacesStayMinimal(t *testing.T) {
	tests := []struct {
		name   string
		typeOf reflect.Type
		method string
	}{
		{"Indexer", reflect.TypeFor[vectorstore.Indexer](), "Index"},
		{"Searcher", reflect.TypeFor[vectorstore.Searcher](), "Search"},
		{"IDDeleter", reflect.TypeFor[vectorstore.IDDeleter](), "DeleteIDs"},
		{"FilterDeleter", reflect.TypeFor[vectorstore.FilterDeleter](), "DeleteWhere"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.typeOf.NumMethod() != 1 || tc.typeOf.Method(0).Name != tc.method {
				t.Fatalf("methods = %v, want only %s", tc.typeOf, tc.method)
			}
		})
	}
}
