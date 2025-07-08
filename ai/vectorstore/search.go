package vectorstore

import (
	"github.com/Tangerg/lynx/ai/vectorstore/filter"
)

const (
	SimilarityThresholdAcceptAll = 0.0
	DefaultTopK                  = 5
)

type SearchRequest struct {
	query               string
	topK                int
	similarityThreshold float64
	filterExpression    *filter.Expression
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

func (s *SearchRequest) FilterExpression() *filter.Expression {
	return s.filterExpression
}
