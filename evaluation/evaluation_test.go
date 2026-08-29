package evaluation_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/Tangerg/scope/core/metadata"
	"github.com/Tangerg/scope/evaluation"
)

func testMetric(name string) evaluation.Metric {
	return evaluation.Metric{Name: evaluation.MetricName(name)}
}

func scoredReport(name string, verdict evaluation.Verdict, score evaluation.Score) evaluation.Report {
	return evaluation.Report{Metric: testMetric(name), Verdict: verdict, Score: &score}
}

func TestCompositeUsesExplicitWeightsAndPassPolicy(t *testing.T) {
	firstMetadata := metadata.Map{}
	if err := firstMetadata.Set("source", "first"); err != nil {
		t.Fatal(err)
	}
	components := []evaluation.Component[string]{
		{Evaluator: evaluation.EvaluatorFunc[string](func(context.Context, string) (evaluation.Report, error) {
			report := scoredReport("quality", evaluation.VerdictPass, 1)
			report.Feedback, report.Metadata = "good", firstMetadata
			return report, nil
		}), Weight: 3, Required: true},
		{Evaluator: evaluation.EvaluatorFunc[string](func(context.Context, string) (evaluation.Report, error) {
			report := scoredReport("style", evaluation.VerdictFail, 0.5)
			report.Feedback = "weak"
			return report, nil
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
	if result.Verdict != evaluation.VerdictPass || result.Score == nil || *result.Score != 0.875 || result.Feedback != "good\n\nweak" {
		t.Fatalf("result = %#v", result)
	}
	if result.Metric.Name != evaluation.MetricNameComposite || len(result.Details) != 2 {
		t.Fatalf("composite identity/details = %#v", result)
	}
	type componentIdentity struct {
		Metric   evaluation.Metric `json:"metric"`
		Weight   float64           `json:"weight"`
		Required bool              `json:"required"`
	}
	identity, found, err := result.Metric.Parameters.Decode[[]componentIdentity]("components")
	if err != nil || !found || len(identity) != 2 || identity[0].Metric.Name != "quality" || identity[0].Weight != 3 || !identity[0].Required {
		t.Fatalf("component identity = (%#v, %v, %v)", identity, found, err)
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
		{Components: []evaluation.Component[string]{{Evaluator: validStringEvaluator()}}, PassPolicy: evaluation.PassAll, MinimumPassed: 1},
		{Components: []evaluation.Component[string]{{Evaluator: validStringEvaluator()}}, PassPolicy: evaluation.PassAny, MinimumPassed: 1},
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
			return scoredReport(name, evaluation.VerdictPass, 1), nil
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
			verdict := evaluation.VerdictFail
			if value > 0 {
				verdict = evaluation.VerdictPass
			}
			return scoredReport("positive", verdict, 1), nil
		}),
		func(value aggregate) (int, error) { return value.Value, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	report, err := projected.Evaluate(t.Context(), aggregate{Value: 3})
	if err != nil || report.Verdict != evaluation.VerdictPass {
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
		verdict, err := score.Verdict(0.5)
		if err != nil {
			return evaluation.Report{}, err
		}
		return evaluation.Report{Metric: testMetric("quality"), Verdict: verdict, Score: &score}, nil
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
		Metrics: []evaluation.MetricSummary{{
			Metric: testMetric("quality"), Evaluated: 2, Passed: 1, Failed: 1,
			Scores: evaluation.Distribution{Count: 2, Mean: 0.5, Minimum: 0, P10: 0, P50: 0, P90: 1, Maximum: 1},
		}},
	}
	if !reflect.DeepEqual(report.Summary, want) {
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
		return scoredReport("quality", evaluation.VerdictPass, 1), nil
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
	if !errors.Is(report.Cases[1].Err, evaluation.ErrCaseNotEvaluated) || !errors.Is(report.Cases[2].Err, evaluation.ErrCaseNotEvaluated) {
		t.Fatalf("pending case errors = (%v, %v)", report.Cases[1].Err, report.Cases[2].Err)
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
		return scoredReport("quality", evaluation.VerdictPass, 1), nil
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

func TestSuitePreservesHeterogeneousResultsAndRunnerSummarizesEachMetric(t *testing.T) {
	latencyMetric, err := evaluation.NewMetric(evaluation.MetricConfig{
		Namespace: "runtime", Name: "latency", Unit: "ms", Direction: evaluation.DirectionLowerIsBetter,
	})
	if err != nil {
		t.Fatal(err)
	}
	suite, err := evaluation.NewSuiteEvaluator(evaluation.SuiteConfig[string]{
		Evaluators: []evaluation.Evaluator[string]{
			evaluation.EvaluatorFunc[string](func(context.Context, string) (evaluation.Report, error) {
				return scoredReport("quality", evaluation.VerdictPass, 0.8), nil
			}),
			evaluation.EvaluatorFunc[string](func(context.Context, string) (evaluation.Report, error) {
				latency := 125.0
				return evaluation.Report{Metric: latencyMetric, Measurement: &latency}, nil
			}),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	report, err := suite.Evaluate(t.Context(), "subject")
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != evaluation.VerdictPass || report.Score != nil || len(report.Details) != 2 {
		t.Fatalf("suite report = %#v", report)
	}
	metrics, found, err := report.Metric.Parameters.Decode[[]evaluation.Metric]("metrics")
	if err != nil || !found || len(metrics) != 2 || metrics[0].Name != "quality" || metrics[1].Name != "latency" {
		t.Fatalf("suite metric identity = (%#v, %v, %v)", metrics, found, err)
	}

	runner, err := evaluation.NewRunner(suite, evaluation.RunnerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	run, err := runner.Run(t.Context(), []evaluation.Case[string]{{ID: "case", Subject: "subject"}})
	if err != nil {
		t.Fatal(err)
	}
	if run.Summary.Total != 1 || run.Summary.Evaluated != 1 || run.Summary.Passed != 1 || len(run.Summary.Metrics) != 3 {
		t.Fatalf("summary = %#v", run.Summary)
	}
	quality := run.Summary.Metrics[1]
	latency := run.Summary.Metrics[2]
	if quality.Metric.Name != "quality" || quality.Scores.Count != 1 || quality.Scores.Mean != 0.8 {
		t.Fatalf("quality summary = %#v", quality)
	}
	if !reflect.DeepEqual(latency.Metric, latencyMetric) || latency.Unjudged != 1 || latency.Measurements.Count != 1 || latency.Measurements.Mean != 125 {
		t.Fatalf("latency summary = %#v", latency)
	}
}

func TestCompositeRejectsReportsThatCannotBeMeaningfullyAggregated(t *testing.T) {
	metric, err := evaluation.NewMetric(evaluation.MetricConfig{Name: "latency", Unit: "ms", Direction: evaluation.DirectionLowerIsBetter})
	if err != nil {
		t.Fatal(err)
	}
	composite, err := evaluation.NewCompositeEvaluator(evaluation.CompositeConfig[string]{
		Components: []evaluation.Component[string]{{Evaluator: evaluation.EvaluatorFunc[string](func(context.Context, string) (evaluation.Report, error) {
			measurement := 10.0
			return evaluation.Report{Metric: metric, Measurement: &measurement}, nil
		})}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := composite.Evaluate(t.Context(), "subject"); !errors.Is(err, evaluation.ErrInvalidReport) {
		t.Fatalf("measurement-only component error = %v", err)
	}
}

func TestScoreMetricAndReportValidation(t *testing.T) {
	parameters := metadata.Map{}
	if err := parameters.Set("rubric", "strict"); err != nil {
		t.Fatal(err)
	}
	metric, err := evaluation.NewMetric(evaluation.MetricConfig{
		Namespace: "custom", Name: "quality", Parameters: parameters,
	})
	if err != nil {
		t.Fatal(err)
	}
	parameters["rubric"][1] = 'X'
	if metric.String() != "custom/quality" || string(metric.Parameters["rubric"]) != `"strict"` {
		t.Fatalf("metric = %#v", metric)
	}
	for _, score := range []float64{-0.1, 1.1, math.NaN(), math.Inf(1)} {
		value := evaluation.Score(score)
		if err := (evaluation.Report{Metric: testMetric("quality"), Score: &value}).Validate(); !errors.Is(err, evaluation.ErrInvalidReport) {
			t.Fatalf("score %v error = %v", score, err)
		}
	}
	value := evaluation.Score(0.5)
	if err := (evaluation.Report{Score: &value}).Validate(); !errors.Is(err, evaluation.ErrInvalidMetric) {
		t.Fatalf("missing metric error = %v", err)
	}
	if _, err := evaluation.NewMetric(evaluation.MetricConfig{Namespace: "bad/name", Name: "quality"}); !errors.Is(err, evaluation.ErrInvalidMetric) {
		t.Fatalf("invalid metric error = %v", err)
	}
	measurement := math.NaN()
	if err := (evaluation.Report{Metric: testMetric("latency"), Measurement: &measurement}).Validate(); !errors.Is(err, evaluation.ErrInvalidReport) {
		t.Fatalf("invalid measurement error = %v", err)
	}
	if err := (evaluation.Report{Metric: testMetric("quality"), Verdict: "maybe"}).Validate(); !errors.Is(err, evaluation.ErrInvalidReport) {
		t.Fatalf("invalid verdict error = %v", err)
	}
	if err := (evaluation.Report{Metric: testMetric("empty")}).Validate(); !errors.Is(err, evaluation.ErrInvalidReport) {
		t.Fatalf("empty report error = %v", err)
	}
}

func validStringEvaluator() evaluation.Evaluator[string] {
	return evaluation.EvaluatorFunc[string](func(context.Context, string) (evaluation.Report, error) {
		return scoredReport("valid", evaluation.VerdictPass, 1), nil
	})
}
