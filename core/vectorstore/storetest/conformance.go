package storetest

import (
	"errors"
	"testing"

	"github.com/Tangerg/scope/core/document"
	"github.com/Tangerg/scope/core/vectorstore"
)

// Capabilities is the exact interface and search-semantics set a backend
// promises. A false interface flag forbids accidental implementation; a false
// HybridSearch flag requires rejection before external I/O.
type Capabilities struct {
	Indexer       bool
	Searcher      bool
	HybridSearch  bool
	IDDeleter     bool
	FilterDeleter bool
}

// Run verifies the backend's exact capability set and the common operations
// that must complete before external I/O. Pass a non-nil zero-value *Store;
// the calls below must not reach provider dependencies.
func Run(t *testing.T, store any, expected Capabilities) {
	t.Helper()
	if store == nil {
		t.Fatal("conformance: store must not be nil")
	}

	indexer, hasIndexer := store.(vectorstore.Indexer)
	searcher, hasSearcher := store.(vectorstore.Searcher)
	idDeleter, hasIDDeleter := store.(vectorstore.IDDeleter)
	filterDeleter, hasFilterDeleter := store.(vectorstore.FilterDeleter)

	assertCapability(t, "Indexer", hasIndexer, expected.Indexer)
	assertCapability(t, "Searcher", hasSearcher, expected.Searcher)
	assertCapability(t, "IDDeleter", hasIDDeleter, expected.IDDeleter)
	assertCapability(t, "FilterDeleter", hasFilterDeleter, expected.FilterDeleter)

	ctx := t.Context()
	if expected.Indexer && hasIndexer {
		indexCases := []struct {
			name string
			docs []*document.Document
			want error
		}{
			{name: "empty documents", want: vectorstore.ErrEmptyDocuments},
			{name: "nil document", docs: []*document.Document{nil}, want: vectorstore.ErrInvalidDocument},
			{name: "missing document ID", docs: []*document.Document{{Text: "content"}}, want: vectorstore.ErrMissingDocumentID},
			{
				name: "duplicate document ID",
				docs: []*document.Document{{ID: "duplicate", Text: "one"}, {ID: "duplicate", Text: "two"}},
				want: vectorstore.ErrDuplicateDocumentID,
			},
		}
		for _, test := range indexCases {
			t.Run("IndexRejects"+test.name+"BeforeIO", func(t *testing.T) {
				request := &vectorstore.IndexRequest{Documents: test.docs}
				if err := indexer.Index(ctx, request); !errors.Is(err, test.want) {
					t.Fatalf("Index() error = %v, want %v", err, test.want)
				}
			})
		}
	}
	if expected.Searcher && hasSearcher {
		t.Run("SearchRejectsInvalidRequestBeforeIO", func(t *testing.T) {
			if _, err := searcher.Search(ctx, &vectorstore.SearchRequest{}); err == nil {
				t.Fatal("Search(zero request) error = nil, want validation error")
			}
		})
		if !expected.HybridSearch {
			t.Run("SearchRejectsUnsupportedHybridBeforeIO", func(t *testing.T) {
				request := &vectorstore.SearchRequest{
					Query: "query", Options: vectorstore.SearchOptions{Mode: vectorstore.SearchModeHybrid},
				}
				if _, err := searcher.Search(ctx, request); !errors.Is(err, vectorstore.ErrUnsupportedSearchMode) {
					t.Fatalf("Search(hybrid) error = %v, want %v", err, vectorstore.ErrUnsupportedSearchMode)
				}
			})
		}
	}
	if expected.IDDeleter && hasIDDeleter {
		t.Run("DeleteIDsTreatsEmptyInputAsNoop", func(t *testing.T) {
			if err := idDeleter.DeleteIDs(ctx, nil); err != nil {
				t.Fatalf("DeleteIDs(nil) error = %v, want nil", err)
			}
		})
	}
	if expected.FilterDeleter && hasFilterDeleter {
		t.Run("DeleteWhereRejectsMissingFilterBeforeIO", func(t *testing.T) {
			if err := filterDeleter.DeleteWhere(ctx, nil); !errors.Is(err, vectorstore.ErrMissingFilter) {
				t.Fatalf("DeleteWhere(nil) error = %v, want %v", err, vectorstore.ErrMissingFilter)
			}
		})
	}
}

func assertCapability(t *testing.T, name string, got, want bool) {
	t.Helper()
	if got != want {
		t.Errorf("%s capability = %t, want %t", name, got, want)
	}
}
