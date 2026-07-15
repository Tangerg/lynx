package vectorstore_test

import (
	"context"
	"errors"
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

type fakeIndexer struct {
	docs []*document.Document
	err  error
}

func (i *fakeIndexer) Add(_ context.Context, docs []*document.Document) error {
	i.docs = docs
	return i.err
}

func TestNewDocumentWriterDelegatesToIndexer(t *testing.T) {
	i := &fakeIndexer{}
	w := vectorstore.NewDocumentWriter(i)

	doc, _ := document.NewDocument("hi", nil)
	if err := w.Write(context.Background(), []*document.Document{doc}); err != nil {
		t.Fatal(err)
	}
	if len(i.docs) != 1 {
		t.Fatalf("indexer received %d docs, want 1", len(i.docs))
	}
}

func TestNewDocumentWriterPropagatesError(t *testing.T) {
	want := errors.New("boom")
	i := &fakeIndexer{err: want}
	w := vectorstore.NewDocumentWriter(i)

	doc, _ := document.NewDocument("hi", nil)
	if err := w.Write(context.Background(), []*document.Document{doc}); !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
}

func TestNewDocumentWriterRejectsEmptyDocs(t *testing.T) {
	i := &fakeIndexer{}
	w := vectorstore.NewDocumentWriter(i)

	if err := w.Write(context.Background(), nil); !errors.Is(err, vectorstore.ErrEmptyDocuments) {
		t.Fatalf("err = %v, want ErrEmptyDocuments", err)
	}
	if i.docs != nil {
		t.Fatal("indexer should not be called when validation fails")
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
