package evaluation

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/samber/lo"

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
}

func NewCompositeEvaluator[T any](config CompositeConfig[T]) (*CompositeEvaluator[T], error) {
	if len(config.Components) == 0 {
		return nil, fmt.Errorf("%w: at least one component is required", ErrInvalidEvaluatorConfig)
	}
	if config.MaxConcurrency < 0 {
		return nil, fmt.Errorf("%w: maximum concurrency must not be negative", ErrInvalidEvaluatorConfig)
	}

	components := make([]Component[T], len(config.Components))
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
	}

	policy := config.PassPolicy
	if policy == "" {
		policy = PassAll
	}
	minimumPassed := config.MinimumPassed
	switch policy {
	case PassAll:
		if minimumPassed != 0 {
			return nil, fmt.Errorf("%w: minimum passed is only valid with the at_least policy", ErrInvalidEvaluatorConfig)
		}
		minimumPassed = len(components)
	case PassAny:
		if minimumPassed != 0 {
			return nil, fmt.Errorf("%w: minimum passed is only valid with the at_least policy", ErrInvalidEvaluatorConfig)
		}
		minimumPassed = 1
	case PassAtLeast:
		if minimumPassed <= 0 || minimumPassed > len(components) {
			return nil, fmt.Errorf("%w: minimum passed must be between 1 and %d", ErrInvalidEvaluatorConfig, len(components))
		}
	default:
		return nil, fmt.Errorf("%w: unsupported pass policy %q", ErrInvalidEvaluatorConfig, policy)
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
		maxConcurrency: maxConcurrency,
	}, nil
}

func (composite *CompositeEvaluator[T]) Evaluate(ctx context.Context, subject T) (Report, error) {
	evaluators := make([]Evaluator[T], len(composite.components))
	for index, component := range composite.components {
		evaluators[index] = component.Evaluator
	}
	reports, err := evaluateAll(ctx, evaluators, composite.maxConcurrency, subject)
	if err != nil {
		return Report{}, err
	}
	for index, report := range reports {
		if report.Score == nil || !report.Verdict.Decided() {
			return Report{}, fmt.Errorf("evaluation: component %d: %w: composite components require a score and verdict", index, ErrInvalidReport)
		}
	}
	return composite.combine(reports)
}

func (composite *CompositeEvaluator[T]) combine(reports []Report) (Report, error) {
	metric, err := composite.metricFor(reports)
	if err != nil {
		return Report{}, err
	}
	combined := Report{Metric: metric, Verdict: VerdictPass, Details: reports}
	feedback := make([]string, 0, len(reports))
	passed := 0
	totalWeight := 0.0
	weightedScore := 0.0
	for index, report := range reports {
		component := composite.components[index]
		if report.Verdict == VerdictPass {
			passed++
		} else if component.Required {
			combined.Verdict = VerdictFail
		}
		weightedScore += report.Score.Float64() * component.Weight
		totalWeight += component.Weight
		if report.Feedback != "" {
			feedback = append(feedback, report.Feedback)
		}
	}
	if passed < composite.minimumPassed {
		combined.Verdict = VerdictFail
	}
	score := Score(weightedScore / totalWeight)
	combined.Score = &score
	combined.Feedback = strings.Join(feedback, "\n\n")
	if err := combined.Validate(); err != nil {
		return Report{}, err
	}
	return combined, nil
}

type componentIdentity struct {
	Metric   Metric  `json:"metric"`
	Weight   float64 `json:"weight"`
	Required bool    `json:"required,omitzero"`
}

func (composite *CompositeEvaluator[T]) metricFor(reports []Report) (Metric, error) {
	components := make([]componentIdentity, len(reports))
	for index, report := range reports {
		components[index] = componentIdentity{
			Metric: report.Metric.Clone(), Weight: composite.components[index].Weight,
			Required: composite.components[index].Required,
		}
	}
	parameters := metadata.Map{}
	for key, value := range map[string]any{
		"components":     components,
		"pass_policy":    composite.passPolicy,
		"minimum_passed": composite.minimumPassed,
	} {
		if err := parameters.Set(key, value); err != nil {
			return Metric{}, fmt.Errorf("evaluation: composite metric identity: %w", err)
		}
	}
	metric, err := NewMetric(MetricConfig{Name: MetricNameComposite, Parameters: parameters})
	if err != nil {
		return Metric{}, fmt.Errorf("evaluation: composite metric identity: %w", err)
	}
	return metric, nil
}
