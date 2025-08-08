package vectorstore

import (
	"github.com/Tangerg/lynx/ai/vectorstore/filter/ast"
)

const (
	SimilarityThresholdAcceptAll = 0.0
	DefaultTopK                  = 5
)

type SearchRequest struct {
	Query               string
	TopK                int
	SimilarityThreshold float64
	Filter              ast.Expr
}

func NewSearchRequest(query string) *SearchRequest {
	return &SearchRequest{
		Query:               query,
		TopK:                DefaultTopK,
		SimilarityThreshold: SimilarityThresholdAcceptAll,
	}
}
