package evaluation

import (
	"context"
	"errors"
	"fmt"
)

var _ Evaluator = (*CompositeEvaluator)(nil)

// CompositeEvaluator runs every child evaluator sequentially against
// the same request and merges their verdicts via mergeResponses —
// AND-of-Pass, average-of-Score, namespace-prefixed metadata.
//
// Use it when one generation should pass multiple criteria (fact
// check + relevancy + style) before being accepted.
//
// Example:
//
//	composite, _ := evaluation.NewCompositeEvaluator(factCheck, relevancy)
//	resp, err := composite.Evaluate(ctx, req)
type CompositeEvaluator struct {
	evaluators []Evaluator
}

func NewCompositeEvaluator(evaluators ...Evaluator) (*CompositeEvaluator, error) {
	if len(evaluators) == 0 {
		return nil, errors.New("evaluation.NewCompositeEvaluator: at least one evaluator is required")
	}
	return &CompositeEvaluator{evaluators: evaluators}, nil
}

func (c *CompositeEvaluator) Evaluate(ctx context.Context, req *Request) (*Response, error) {
	if req == nil {
		return nil, errors.New("evaluation.CompositeEvaluator.Evaluate: request must not be nil")
	}

	responses := make([]*Response, 0, len(c.evaluators))
	for i, evaluator := range c.evaluators {
		resp, err := evaluator.Evaluate(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("evaluation.CompositeEvaluator: child #%d: %w", i, err)
		}
		responses = append(responses, resp)
	}
	return mergeResponses(responses)
}
