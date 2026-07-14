package evaluation

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/metadata"
)

// Composite evaluates children sequentially, applies AND semantics to Pass,
// and averages normalized scores. It is immutable after construction.
type Composite struct {
	evaluators []Evaluator
}

// NewComposite snapshots evaluators. At least one non-nil evaluator is
// required.
func NewComposite(evaluators ...Evaluator) (*Composite, error) {
	if len(evaluators) == 0 {
		return nil, fmt.Errorf("%w: at least one evaluator is required", ErrInvalidConfig)
	}
	snapshot := make([]Evaluator, len(evaluators))
	for i, evaluator := range evaluators {
		if evaluator == nil {
			return nil, fmt.Errorf("%w: evaluators[%d] is nil", ErrInvalidConfig, i)
		}
		snapshot[i] = evaluator
	}
	return &Composite{evaluators: snapshot}, nil
}

// Evaluate stops on the first child error or invalid child result. Error
// wrapping preserves errors.Is identities.
func (c *Composite) Evaluate(ctx context.Context, request Request) (Result, error) {
	results := make([]Result, 0, len(c.evaluators))
	for i, evaluator := range c.evaluators {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		result, err := evaluator.Evaluate(ctx, request)
		if err != nil {
			return Result{}, fmt.Errorf("evaluation: evaluator %d: %w", i, err)
		}
		if err := result.Validate(); err != nil {
			return Result{}, fmt.Errorf("evaluation: evaluator %d: %w", i, err)
		}
		results = append(results, result)
	}
	return merge(results)
}

func merge(results []Result) (Result, error) {
	if len(results) == 0 {
		return Result{}, fmt.Errorf("%w: no results to merge", ErrInvalidResult)
	}
	if len(results) == 1 {
		result := results[0]
		result.Metadata = result.Metadata.Clone()
		return result, nil
	}

	merged := Result{Pass: true, Metadata: metadata.New()}
	feedback := make([]string, 0, len(results))
	passed := 0
	for i, result := range results {
		merged.Pass = merged.Pass && result.Pass
		merged.Score += result.Score
		if result.Pass {
			passed++
		}
		if result.Feedback != "" {
			feedback = append(feedback, fmt.Sprintf("[Evaluation %d] %s", i+1, result.Feedback))
		}
		for key, value := range result.Metadata {
			merged.Metadata[fmt.Sprintf("evaluation_%d_%s", i+1, key)] = append([]byte(nil), value...)
		}
	}
	merged.Score /= float64(len(results))
	merged.Feedback = strings.Join(feedback, "\n\n")
	if err := metadata.Set(merged.Metadata, "total_evaluations", len(results)); err != nil {
		return Result{}, err
	}
	if err := metadata.Set(merged.Metadata, "passed_count", passed); err != nil {
		return Result{}, err
	}
	return merged, nil
}
