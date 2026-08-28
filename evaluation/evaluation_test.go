package evaluation_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/Tangerg/scope/core/metadata"
	"github.com/Tangerg/scope/evaluation"
)

func testMetric(name string) evaluation.Metric {
	return evaluation.Metric{Name: evaluation.MetricName(name)}
}

func TestCompositeUsesExplicitWeightsAndPassPolicy(t *testing.T) {
	firstMetadata := metadata.Map{}
	if err := firstMetadata.Set("source", "first"); err != nil {
		t.Fatal(err)
	}
	components := []evaluation.Component[string]{
		{Evaluator: evaluation.EvaluatorFunc[string](func(context.Context, string) (evaluation.Report, error) {
			return evaluation.Report{
				Metric: testMetric("quality"), Passed: true, Score: 1, Feedback: "good", Metadata: firstMetadata,
			}, nil
		}), Weight: 3, Required: true},
		{Evaluator: evaluation.EvaluatorFunc[string](func(context.Context, string) (evaluation.Report, error) {
			return evaluation.Report{Metric: testMetric("style"), Passed: false, Score: 0.5, Feedback: "weak"}, nil
		}), Weight: 1},
	}
	composite, err := evaluation.NewCompositeEvaluator(evaluation.CompositeConfig[string]{
		Components: components, PassPolicy: evaluation.PassAtLeast, MinimumPassed: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	components[0].Evaluator = nil
	result, err := composite.Evaluate(t.Context(), "subject")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Passed || result.Score != 0.875 || result.Feedback != "good\n\nweak" {
		t.Fatalf("result = %#v", result)
	}
	if result.Metric.Name != evaluation.MetricNameComposite || len(result.Details) != 2 {
		t.Fatalf("composite identity/details = %#v", result)
	}
	result.Details[0].Metadata["source"][1] = 'X'
	if string(firstMetadata["source"]) != "\"first\"" {
		t.Fatalf("child metadata was aliased: %s", firstMetadata["source"])
	}
}

func TestCompositeValidatesConfigurationAndPreservesErrors(t *testing.T) {
	if _, err := evaluation.NewCompositeEvaluator(evaluation.CompositeConfig[string]{}); !errors.Is(err, evaluation.ErrInvalidEvaluatorConfig) {
		t.Fatalf("empty composite error = %v", err)
	}
	for _, config := range []evaluation.CompositeConfig[string]{
		{Components: []evaluation.Component[string]{{}}},
		{Components: []evaluation.Component[string]{{Evaluator: validStringEvaluator(), Weight: -1}}},
		{Components: []evaluation.Component[string]{{Evaluator: validStringEvaluator(), Weight: math.NaN()}}},
		{Components: []evaluation.Component[string]{{Evaluator: validStringEvaluator(), Weight: math.Inf(1)}}},
		{Components: []evaluation.Component[string]{{Evaluator: validStringEvaluator()}}, PassPolicy: "unsupported"},
		{Components: []evaluation.Component[string]{{Evaluator: validStringEvaluator()}}, PassPolicy: evaluation.PassAtLeast},
		{Components: []evaluation.Component[string]{{Evaluator: validStringEvaluator()}}, MaxConcurrency: -1},
	} {
		if _, err := evaluation.NewCompositeEvaluator(config); !errors.Is(err, evaluation.ErrInvalidEvaluatorConfig) {
			t.Fatalf("NewCompositeEvaluator(%#v) error = %v", config, err)
		}
	}

	childErr := errors.New("child failed")
	composite, err := evaluation.NewCompositeEvaluator(evaluation.CompositeConfig[string]{
		Components: []evaluation.Component[string]{{Evaluator: evaluation.EvaluatorFunc[string](
			func(context.Context, string) (evaluation.Report, error) { return evaluation.Report{}, childErr },
		)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := composite.Evaluate(t.Context(), "subject"); !errors.Is(err, childErr) {
		t.Fatalf("child error = %v", err)
	}
}

func TestCompositeRunsIndependentComponentsConcurrently(t *testing.T) {
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	evaluator := func(name string) evaluation.Evaluator[string] {
		return evaluation.EvaluatorFunc[string](func(context.Context, string) (evaluation.Report, error) {
			started <- struct{}{}
			<-release
			return evaluation.Report{Metric: testMetric(name), Passed: true, Score: 1}, nil
		})
	}
	composite, err := evaluation.NewCompositeEvaluator(evaluation.CompositeConfig[string]{
		Components:     []evaluation.Component[string]{{Evaluator: evaluator("one")}, {Evaluator: evaluator("two")}},
		MaxConcurrency: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, evaluateErr := composite.Evaluate(t.Context(), "subject")
		done <- evaluateErr
	}()
	<-started
	<-started
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestProjectionAdaptsAggregateCases(t *testing.T) {
	type aggregate struct{ Value int }
	projected, err := evaluation.NewProjectionEvaluator(
		evaluation.EvaluatorFunc[int](func(_ context.Context, value int) (evaluation.Report, error) {
			return evaluation.Report{Metric: testMetric("positive"), Passed: value > 0, Score: 1}, nil
		}),
		func(value aggregate) (int, error) { return value.Value, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	report, err := projected.Evaluate(t.Context(), aggregate{Value: 3})
	if err != nil || !report.Passed {
		t.Fatalf("projected report = %#v, %v", report, err)
	}
}

func TestRunnerCollectsCasesAndBuildsDistribution(t *testing.T) {
	evaluator := evaluation.EvaluatorFunc[float64](func(_ context.Context, value float64) (evaluation.Report, error) {
		if value < 0 {
			return evaluation.Report{}, errors.New("invalid subject")
		}
		score, err := evaluation.NewScore(value)
		if err != nil {
			return evaluation.Report{}, err
		}
		return evaluation.Report{Metric: testMetric("quality"), Passed: score >= 0.5, Score: score}, nil
	})
	runner, err := evaluation.NewRunner(evaluator, evaluation.RunnerConfig{MaxConcurrency: 2})
	if err != nil {
		t.Fatal(err)
	}
	cases := []evaluation.Case[float64]{
		{ID: "good", Subject: 1},
		{ID: "bad", Subject: 0},
		{ID: "error", Subject: -1},
	}
	report, err := runner.Run(t.Context(), cases)
	if err != nil {
		t.Fatal(err)
	}
	want := evaluation.Summary{
		Total: 3, Evaluated: 2, Passed: 1, Failed: 1, Errors: 1,
		Mean: 0.5, Minimum: 0, P10: 0, P50: 0, P90: 1, Maximum: 1,
	}
	if report.Summary != want {
		t.Fatalf("summary = %#v, want %#v", report.Summary, want)
	}
	if report.Cases[0].ID != "good" || report.Cases[2].Err == nil {
		t.Fatalf("case order/results = %#v", report.Cases)
	}

	duplicate := append(cases, evaluation.Case[float64]{ID: "good"})
	if _, err := runner.Run(t.Context(), duplicate); !errors.Is(err, evaluation.ErrInvalidCase) {
		t.Fatalf("duplicate case error = %v", err)
	}
}

func TestRunnerFailFastPreservesCaseIdentityAndStopsScheduling(t *testing.T) {
	firstErr := errors.New("first failed")
	calls := 0
	evaluator := evaluation.EvaluatorFunc[int](func(_ context.Context, value int) (evaluation.Report, error) {
		calls++
		if value == 1 {
			return evaluation.Report{}, firstErr
		}
		return evaluation.Report{Metric: testMetric("quality"), Passed: true, Score: 1}, nil
	})
	runner, err := evaluation.NewRunner(evaluator, evaluation.RunnerConfig{
		MaxConcurrency: 1,
		ErrorPolicy:    evaluation.ErrorFailFast,
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := runner.Run(t.Context(), []evaluation.Case[int]{
		{ID: "first", Subject: 1},
		{ID: "second", Subject: 2},
		{ID: "third", Subject: 3},
	})
	if !errors.Is(err, firstErr) {
		t.Fatalf("Run error = %v, want first failure", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
	for index, id := range []string{"first", "second", "third"} {
		if report.Cases[index].ID != id || report.Cases[index].Err == nil {
			t.Fatalf("cases[%d] = %#v", index, report.Cases[index])
		}
	}
	if report.Summary.Total != 3 || report.Summary.Errors != 3 || report.Summary.Evaluated != 0 {
		t.Fatalf("summary = %#v", report.Summary)
	}
}

func TestRunnerDefaultConcurrencyIsBounded(t *testing.T) {
	started := make(chan struct{}, evaluation.DefaultMaxConcurrency+1)
	release := make(chan struct{})
	evaluator := evaluation.EvaluatorFunc[int](func(_ context.Context, _ int) (evaluation.Report, error) {
		started <- struct{}{}
		<-release
		return evaluation.Report{Metric: testMetric("quality"), Passed: true, Score: 1}, nil
	})
	runner, err := evaluation.NewRunner(evaluator, evaluation.RunnerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	cases := make([]evaluation.Case[int], evaluation.DefaultMaxConcurrency+1)
	for index := range cases {
		cases[index] = evaluation.Case[int]{ID: fmt.Sprintf("case-%d", index)}
	}
	done := make(chan error, 1)
	go func() {
		_, runErr := runner.Run(t.Context(), cases)
		done <- runErr
	}()
	for range evaluation.DefaultMaxConcurrency {
		<-started
	}
	select {
	case <-started:
		t.Fatal("default concurrency exceeded its fixed bound")
	default:
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestScoreMetricAndReportValidation(t *testing.T) {
	parameters := metadata.Map{}
	if err := parameters.Set("cutoff", 5); err != nil {
		t.Fatal(err)
	}
	metric, err := evaluation.NewMetric("retrieval", "precision", parameters)
	if err != nil {
		t.Fatal(err)
	}
	parameters["cutoff"][0] = '9'
	if metric.String() != "retrieval/precision" || string(metric.Parameters["cutoff"]) != "5" {
		t.Fatalf("metric = %#v", metric)
	}
	for _, score := range []float64{-0.1, 1.1, math.NaN(), math.Inf(1)} {
		if err := (evaluation.Report{Metric: testMetric("quality"), Score: evaluation.Score(score)}).Validate(); !errors.Is(err, evaluation.ErrInvalidReport) {
			t.Fatalf("score %v error = %v", score, err)
		}
	}
	if err := (evaluation.Report{Score: 0.5}).Validate(); !errors.Is(err, evaluation.ErrInvalidMetric) {
		t.Fatalf("missing metric error = %v", err)
	}
	if _, err := evaluation.NewMetric("bad/name", "quality", nil); !errors.Is(err, evaluation.ErrInvalidMetric) {
		t.Fatalf("invalid metric error = %v", err)
	}
}

func validStringEvaluator() evaluation.Evaluator[string] {
	return evaluation.EvaluatorFunc[string](func(context.Context, string) (evaluation.Report, error) {
		return evaluation.Report{Metric: testMetric("valid"), Passed: true, Score: 1}, nil
	})
}
