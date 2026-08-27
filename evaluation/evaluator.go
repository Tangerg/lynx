package evaluation

import "context"

// Evaluator evaluates one subject and returns a normalized report.
type Evaluator[T any] interface {
	Evaluate(context.Context, T) (Report, error)
}

// EvaluatorFunc adapts a function to [Evaluator].
type EvaluatorFunc[T any] func(context.Context, T) (Report, error)

func (evaluate EvaluatorFunc[T]) Evaluate(ctx context.Context, subject T) (Report, error) {
	return evaluate(ctx, subject)
}
