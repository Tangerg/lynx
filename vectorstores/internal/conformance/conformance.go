package conformance

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore"
)

// Capabilities is the exact vectorstore interface set a backend promises.
// False means the backend must not accidentally satisfy that capability.
type Capabilities struct {
	Indexer       bool
	Searcher      bool
	IDDeleter     bool
	FilterDeleter bool
}

// Run verifies the backend's exact capability set and the common operations
// that must complete before external I/O. Pass a non-nil zero-value *Store;
// the calls below must not reach provider dependencies.
func Run(t *testing.T, store any, want Capabilities) {
	t.Helper()
	if store == nil {
		t.Fatal("conformance: store must not be nil")
	}

	indexer, hasIndexer := store.(vectorstore.Indexer)
	searcher, hasSearcher := store.(vectorstore.Searcher)
	idDeleter, hasIDDeleter := store.(vectorstore.IDDeleter)
	filterDeleter, hasFilterDeleter := store.(vectorstore.FilterDeleter)

	assertCapability(t, "Indexer", hasIndexer, want.Indexer)
	assertCapability(t, "Searcher", hasSearcher, want.Searcher)
	assertCapability(t, "IDDeleter", hasIDDeleter, want.IDDeleter)
	assertCapability(t, "FilterDeleter", hasFilterDeleter, want.FilterDeleter)

	ctx := context.Background()
	if want.Indexer && hasIndexer {
		t.Run("AddRejectsEmptyDocumentsBeforeIO", func(t *testing.T) {
			if err := indexer.Add(ctx, nil); !errors.Is(err, vectorstore.ErrEmptyDocuments) {
				t.Fatalf("Add(nil) error = %v, want %v", err, vectorstore.ErrEmptyDocuments)
			}
		})
	}
	if want.Searcher && hasSearcher {
		t.Run("SearchRejectsInvalidRequestBeforeIO", func(t *testing.T) {
			if _, err := searcher.Search(ctx, vectorstore.SearchRequest{}); err == nil {
				t.Fatal("Search(zero request) error = nil, want validation error")
			}
		})
	}
	if want.IDDeleter && hasIDDeleter {
		t.Run("DeleteIDsTreatsEmptyInputAsNoop", func(t *testing.T) {
			if err := idDeleter.DeleteIDs(ctx, nil); err != nil {
				t.Fatalf("DeleteIDs(nil) error = %v, want nil", err)
			}
		})
	}
	if want.FilterDeleter && hasFilterDeleter {
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
