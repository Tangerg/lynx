package vectorstore

import (
	"context"
	"github.com/Tangerg/lynx/ai/commons/document"
	"github.com/Tangerg/lynx/ai/vectorstore/filter"
)

type VectorStore interface {
	document.Writer
	Name() string
	Add(ctx context.Context, docs []*document.Document) error
	Delete(ctx context.Context, expression string) error
	DeleteByIDs(ctx context.Context, ids []string) error
	DeleteByFilterExpression(ctx context.Context, expression *filter.Expression) error
	SimilaritySearch(ctx context.Context, query string) ([]*document.Document, error)
	SimilaritySearchBySearchRequest(ctx context.Context, request *SearchRequest) ([]*document.Document, error)
	NativeClient() (any, error)
}
