package vectorstore_test

import (
	"math"
	"reflect"
	"testing"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/vectorstore"
)

func TestSearchRequestValidate(t *testing.T) {
	cases := []struct {
		name string
		req  vectorstore.SearchRequest
		ok   bool
	}{
		{"empty query", vectorstore.SearchRequest{TopK: 5}, false},
		{"zero topk", vectorstore.SearchRequest{Query: "hi"}, false},
		{"out-of-range minscore", vectorstore.SearchRequest{Query: "hi", TopK: 5, MinScore: 1.5}, false},
		{"nan minscore", vectorstore.SearchRequest{Query: "hi", TopK: 5, MinScore: math.NaN()}, false},
		{"valid", vectorstore.SearchRequest{Query: "hi", TopK: 5, MinScore: 0.5}, true},
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

func TestSearchRequestValidateMatches(t *testing.T) {
	request := vectorstore.SearchRequest{Query: "lynx", TopK: 2, MinScore: 0.5}
	first, _ := document.NewDocument("first", nil)
	second, _ := document.NewDocument("second", nil)
	valid := []vectorstore.Match{{Document: first, Score: 0.9}, {Document: second, Score: 0.5}}
	if err := request.ValidateMatches(valid); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		matches []vectorstore.Match
	}{
		{name: "too many", matches: append(valid, vectorstore.Match{Document: second, Score: 0.5})},
		{name: "nil document", matches: []vectorstore.Match{{Score: 0.9}}},
		{name: "out of range", matches: []vectorstore.Match{{Document: first, Score: 1.1}}},
		{name: "below threshold", matches: []vectorstore.Match{{Document: first, Score: 0.4}}},
		{name: "not sorted", matches: []vectorstore.Match{{Document: first, Score: 0.5}, {Document: second, Score: 0.9}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := request.ValidateMatches(test.matches); err == nil {
				t.Fatal("ValidateMatches accepted invalid output")
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
		{"Indexer", reflect.TypeFor[vectorstore.Indexer](), "Add"},
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
