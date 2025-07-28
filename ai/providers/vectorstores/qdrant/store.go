package qdrant

import (
	"context"
	"github.com/Tangerg/lynx/ai/commons/document"
	"github.com/Tangerg/lynx/ai/vectorstore"
	"github.com/Tangerg/lynx/ai/vectorstore/filter"
	"github.com/Tangerg/lynx/ai/vectorstore/filter/ast"
	"github.com/qdrant/go-client/qdrant"
)

var _ vectorstore.Store = (*Store)(nil)

type Store struct {
	client *qdrant.Client
}

func NewStore() (*Store, error) {
	client, err := qdrant.NewClient(&qdrant.Config{
		Host: "localhost",
		Port: 6334,
	})
	if err != nil {
		return nil, err
	}
	return &Store{
		client: client,
	}, nil
}

func (s *Store) Write(ctx context.Context, docs []*document.Document) error {
	return s.Add(ctx, docs)
}

func (s *Store) Name() string {
	return vectorstore.Qdrant
}

func (s *Store) Add(ctx context.Context, docs []*document.Document) error {
	//TODO implement me
	panic("implement me")
}

func (s *Store) Delete(ctx context.Context, expr ast.Expr) error {
	//TODO implement me
	panic("implement me")
}

func (s *Store) DeleteByIDs(ctx context.Context, ids []string) error {
	expr, err := filter.
		NewBuilder().
		In("id", ids).
		Build()
	if err != nil {
		return err
	}

	return s.Delete(ctx, expr)
}

func (s *Store) DeleteByFilter(ctx context.Context, fs string) error {
	expr, err := filter.Parse(fs)
	if err != nil {
		return err
	}

	return s.Delete(ctx, expr)
}

func (s *Store) SimilaritySearch(ctx context.Context, request *vectorstore.SearchRequest) ([]*document.Document, error) {
	//TODO implement me
	panic("implement me")
}

func (s *Store) SimilaritySearchByQuery(ctx context.Context, query string) ([]*document.Document, error) {
	return s.SimilaritySearch(ctx, vectorstore.NewSearchRequest(query))
}

func (s *Store) NativeClient() any {
	return s.client
}
