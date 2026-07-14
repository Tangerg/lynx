package conformance_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/internal/conformance"
)

type allCapabilities struct{}

func (allCapabilities) Add(context.Context, []*document.Document) error {
	return vectorstore.ErrEmptyDocuments
}
func (allCapabilities) Search(context.Context, vectorstore.SearchRequest) ([]vectorstore.Match, error) {
	return nil, vectorstore.SearchRequest{}.Validate()
}
func (allCapabilities) DeleteIDs(context.Context, []string) error { return nil }
func (allCapabilities) DeleteWhere(context.Context, filter.Expr) error {
	return vectorstore.ErrMissingFilter
}

func TestRun(t *testing.T) {
	conformance.Run(t, allCapabilities{}, conformance.Capabilities{
		Indexer: true, Searcher: true, IDDeleter: true, FilterDeleter: true,
	})
}
