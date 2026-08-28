package evaluation

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/samber/lo"
	"golang.org/x/sync/errgroup"

	"github.com/Tangerg/scope/core/metadata"
)

type PassPolicy string

const (
	PassAll     PassPolicy = "all"
	PassAny     PassPolicy = "any"
	PassAtLeast PassPolicy = "at_least"
)

// Component assigns score weight and pass criticality to one evaluator.
// A zero Weight selects 1. Required components must pass independently of the
// aggregate pass policy.
type Component[T any] struct {
	Evaluator Evaluator[T]
	Weight    float64
	Required  bool
}

// CompositeConfig defines score aggregation, pass semantics, and bounded
// concurrency. A zero MaxConcurrency selects DefaultMaxConcurrency.
type CompositeConfig[T any] struct {
	Components     []Component[T]
	PassPolicy     PassPolicy
	MinimumPassed  int
	MaxConcurrency int
}

type CompositeEvaluator[T any] struct {
	components     []Component[T]
	passPolicy     PassPolicy
	minimumPassed  int
	maxConcurrency int
	metric         Metric
}

func NewCompositeEvaluator[T any](config CompositeConfig[T]) (*CompositeEvaluator[T], error) {
	if len(config.Components) == 0 {
		return nil, fmt.Errorf("%w: at least one component is required", ErrInvalidEvaluatorConfig)
	}
	if config.MaxConcurrency < 0 {
		return nil, fmt.Errorf("%w: maximum concurrency must not be negative", ErrInvalidEvaluatorConfig)
	}

	components := make([]Component[T], len(config.Components))
	weights := make([]float64, len(config.Components))
	for index, component := range config.Components {
		if lo.IsNil(component.Evaluator) {
			return nil, fmt.Errorf("%w: components[%d] evaluator is nil", ErrInvalidEvaluatorConfig, index)
		}
		if math.IsNaN(component.Weight) || math.IsInf(component.Weight, 0) || component.Weight < 0 {
			return nil, fmt.Errorf("%w: components[%d] weight must be finite and non-negative", ErrInvalidEvaluatorConfig, index)
		}
		if component.Weight == 0 {
			component.Weight = 1
		}
		components[index] = component
		weights[index] = component.Weight
	}

	policy := config.PassPolicy
	if policy == "" {
		policy = PassAll
	}
	minimumPassed := config.MinimumPassed
	switch policy {
	case PassAll:
		minimumPassed = len(components)
	case PassAny:
		minimumPassed = 1
	case PassAtLeast:
		if minimumPassed <= 0 || minimumPassed > len(components) {
			return nil, fmt.Errorf("%w: minimum passed must be between 1 and %d", ErrInvalidEvaluatorConfig, len(components))
		}
	default:
		return nil, fmt.Errorf("%w: unsupported pass policy %q", ErrInvalidEvaluatorConfig, policy)
	}

	parameters := metadata.Map{}
	if err := parameters.Set("pass_policy", policy); err != nil {
		return nil, fmt.Errorf("%w: composite metric: %w", ErrInvalidEvaluatorConfig, err)
	}
	if err := parameters.Set("minimum_passed", minimumPassed); err != nil {
		return nil, fmt.Errorf("%w: composite metric: %w", ErrInvalidEvaluatorConfig, err)
	}
	if err := parameters.Set("weights", weights); err != nil {
		return nil, fmt.Errorf("%w: composite metric: %w", ErrInvalidEvaluatorConfig, err)
	}
	metric, err := NewMetric("", MetricNameComposite, parameters)
	if err != nil {
		return nil, fmt.Errorf("%w: composite metric: %w", ErrInvalidEvaluatorConfig, err)
	}

	maxConcurrency := config.MaxConcurrency
	if maxConcurrency == 0 {
		maxConcurrency = DefaultMaxConcurrency
	}
	if maxConcurrency > len(components) {
		maxConcurrency = len(components)
	}
	return &CompositeEvaluator[T]{
		components: components, passPolicy: policy, minimumPassed: minimumPassed,
		maxConcurrency: maxConcurrency, metric: metric,
	}, nil
}

func (composite *CompositeEvaluator[T]) Evaluate(ctx context.Context, subject T) (Report, error) {
	reports := make([]Report, len(composite.components))
	group, groupContext := errgroup.WithContext(ctx)
	group.SetLimit(composite.maxConcurrency)
	for index, component := range composite.components {
		group.Go(func() error {
			report, err := component.Evaluator.Evaluate(groupContext, subject)
			if err != nil {
				return fmt.Errorf("evaluation: component %d: %w", index, err)
			}
			if err := report.Validate(); err != nil {
				return fmt.Errorf("evaluation: component %d: %w", index, err)
			}
			reports[index] = report.Clone()
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return Report{}, err
	}
	return composite.combine(reports)
}

func (composite *CompositeEvaluator[T]) combine(reports []Report) (Report, error) {
	combined := Report{Metric: composite.metric.Clone(), Passed: true, Details: reports}
	feedback := make([]string, 0, len(reports))
	passed := 0
	totalWeight := 0.0
	for index, report := range reports {
		component := composite.components[index]
		if report.Passed {
			passed++
		} else if component.Required {
			combined.Passed = false
		}
		combined.Score += Score(float64(report.Score) * component.Weight)
		totalWeight += component.Weight
		if report.Feedback != "" {
			feedback = append(feedback, report.Feedback)
		}
	}
	combined.Passed = combined.Passed && passed >= composite.minimumPassed
	combined.Score = Score(float64(combined.Score) / totalWeight)
	combined.Feedback = strings.Join(feedback, "\n\n")
	if err := combined.Validate(); err != nil {
		return Report{}, err
	}
	return combined, nil
}
