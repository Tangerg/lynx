package storetest_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/core/vectorstore/storetest"
)

type allCapabilities struct{}

func (allCapabilities) Add(context.Context, []*document.Document) error {
	return vectorstore.ErrInvalidDocument
}
func (allCapabilities) Search(context.Context, vectorstore.SearchRequest) ([]vectorstore.Match, error) {
	return nil, vectorstore.SearchRequest{}.Validate()
}
func (allCapabilities) DeleteIDs(context.Context, []string) error { return nil }
func (allCapabilities) DeleteWhere(context.Context, filter.Predicate) error {
	return vectorstore.ErrMissingFilter
}

func TestRun(t *testing.T) {
	storetest.Run(t, validatingCapabilities{}, storetest.Capabilities{
		Indexer: true, Searcher: true, IDDeleter: true, FilterDeleter: true,
	})
}

type validatingCapabilities struct{ allCapabilities }

func (validatingCapabilities) Add(_ context.Context, docs []*document.Document) error {
	switch {
	case len(docs) == 0:
		return vectorstore.ErrEmptyDocuments
	case docs[0] == nil:
		return vectorstore.ErrInvalidDocument
	case docs[0].ID == "":
		return vectorstore.ErrMissingDocumentID
	default:
		return vectorstore.ErrDuplicateDocumentID
	}
}
