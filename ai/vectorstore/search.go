package vectorstore

import (
	"github.com/Tangerg/lynx/ai/vectorstore/filter/ast"
)

const (
	SimilarityThresholdAcceptAll = 0.0
	DefaultTopK                  = 5
)

type SearchRequest struct {
	query               string
	topK                int
	similarityThreshold float64
	expr                ast.ComputedExpr
}

func (s *SearchRequest) Query() string {
	return s.query
}

func (s *SearchRequest) TopK() int {
	return s.topK
}

func (s *SearchRequest) SimilarityThreshold() float64 {
	return s.similarityThreshold
}

func (s *SearchRequest) Expr() ast.ComputedExpr {
	return s.expr
}
