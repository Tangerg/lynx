// Package evaluation defines the [Evaluator] surface used by RAG and
// agent pipelines to score generated responses — relevancy,
// factuality, and any other criteria the application chooses.
// Concrete evaluators ([FactCheckingEvaluator], [RelevancyEvaluator])
// live in this package; combine several into one verdict via
// [CompositeEvaluator].
package evaluation

import "context"

// Evaluator scores an AI response against a request. Implementations
// pick a strategy (relevancy check, fact verification, style scoring,
// ...) and return a [*Response] with the verdict plus any feedback the
// caller should surface.
type Evaluator interface {
	// Evaluate scores the response inside req.
	Evaluate(ctx context.Context, req *Request) (*Response, error)
}
