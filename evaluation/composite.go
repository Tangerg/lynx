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

func (policy PassPolicy) resolve() PassPolicy {
	if policy == "" {
		return PassAll
	}
	return policy
}

func (policy PassPolicy) minimum(componentCount, configured int) (int, error) {
	switch policy.resolve() {
	case PassAll:
		if configured != 0 {
			return 0, fmt.Errorf("%w: minimum passed is only valid with the at_least policy", ErrInvalidEvaluatorConfig)
		}
		return componentCount, nil
	case PassAny:
		if configured != 0 {
			return 0, fmt.Errorf("%w: minimum passed is only valid with the at_least policy", ErrInvalidEvaluatorConfig)
		}
		return 1, nil
	case PassAtLeast:
		if configured <= 0 || configured > componentCount {
			return 0, fmt.Errorf("%w: minimum passed must be between 1 and %d", ErrInvalidEvaluatorConfig, componentCount)
		}
		return configured, nil
	default:
		return 0, fmt.Errorf("%w: unsupported pass policy %q", ErrInvalidEvaluatorConfig, policy)
	}
}

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

	policy := config.PassPolicy.resolve()
	minimumPassed, err := policy.minimum(len(components), config.MinimumPassed)
	if err != nil {
		return nil, err
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

type compositeMetricIdentity struct {
	Components    []componentIdentity `json:"components"`
	PassPolicy    PassPolicy          `json:"pass_policy"`
	MinimumPassed int                 `json:"minimum_passed"`
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
	identity := compositeMetricIdentity{
		Components: components, PassPolicy: composite.passPolicy,
		MinimumPassed: composite.minimumPassed,
	}
	if err := parameters.Set(metricConfigurationKey, identity); err != nil {
		return Metric{}, fmt.Errorf("evaluation: composite metric identity: %w", err)
	}
	metric, err := NewMetric(MetricConfig{Name: MetricNameComposite, Parameters: parameters})
	if err != nil {
		return Metric{}, fmt.Errorf("evaluation: composite metric identity: %w", err)
	}
	return metric, nil
}
