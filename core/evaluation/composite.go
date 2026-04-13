package evaluation

import (
	"context"
	"errors"
)

var _ Evaluator = (*CompositeEvaluator)(nil)

// CompositeEvaluator is a composite pattern implementation that orchestrates multiple
// evaluators to perform comprehensive evaluation. It executes all child evaluators
// sequentially and merges their results into a single consolidated response.
//
// This pattern is useful when you need to:
//   - Apply multiple evaluation criteria to the same generation
//   - Combine different evaluation aspects (e.g., relevance, accuracy, style)
//   - Get an aggregated score and feedback from multiple evaluators
//
// Example use case:
//
//	Evaluating a generated answer with both fact-checking and relevance evaluators
type CompositeEvaluator struct {
	evaluators []Evaluator
}

func NewCompositeEvaluator(evaluators ...Evaluator) (*CompositeEvaluator, error) {
	if len(evaluators) == 0 {
		return nil, errors.New("empty evaluators")
	}

	return &CompositeEvaluator{
		evaluators: evaluators,
	}, nil
}

func (c *CompositeEvaluator) Evaluate(ctx context.Context, req *Request) (*Response, error) {
	if req == nil {
		return nil, errors.New("nil request")
	}

	responses := make([]*Response, 0, len(c.evaluators))

	for _, evaluator := range c.evaluators {
		resp, err := evaluator.Evaluate(ctx, req)
		if err != nil {
			return nil, err
		}
		responses = append(responses, resp)
	}

	return mergeResponses(responses)
}
