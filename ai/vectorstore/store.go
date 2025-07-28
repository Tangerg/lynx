package vectorstore

import (
	"context"
	"github.com/Tangerg/lynx/ai/commons/document"
	"github.com/Tangerg/lynx/ai/vectorstore/filter/ast"
)

type Store interface {
	document.Writer
	Name() string
	Add(ctx context.Context, docs []*document.Document) error
	Delete(ctx context.Context, expr ast.Expr) error
	DeleteByIDs(ctx context.Context, ids []string) error
	DeleteByFilter(ctx context.Context, filter string) error
	SimilaritySearch(ctx context.Context, request *SearchRequest) ([]*document.Document, error)
	SimilaritySearchByQuery(ctx context.Context, query string) ([]*document.Document, error)
	NativeClient() any
}
