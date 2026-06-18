package evaluation

import "context"

// Evaluator scores an AI response against a request. Implementations
// pick a strategy (relevancy check, fact verification, style scoring,
// ...) and return a [*Response] with the verdict plus any feedback the
// caller should surface.
type Evaluator interface {
	Evaluate(ctx context.Context, req *Request) (*Response, error)
}
