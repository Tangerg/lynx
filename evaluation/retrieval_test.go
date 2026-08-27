package evaluation_test

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/Tangerg/scope/evaluation"
)

func TestRetrievalEvaluatorCalculatesRankingMetricsAtCutoff(t *testing.T) {
	sample, err := evaluation.NewRetrievalSample(
		[]string{"a", "b", "c", "d"},
		[]string{"b", "d", "e"},
	)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		metric evaluation.RetrievalMetric
		want   float64
	}{
		{metric: evaluation.RetrievalMetricPrecision, want: 0.5},
		{metric: evaluation.RetrievalMetricRecall, want: 2.0 / 3.0},
		{metric: evaluation.RetrievalMetricReciprocalRank, want: 0.5},
		{
			metric: evaluation.RetrievalMetricNDCG,
			want: (1/math.Log2(3) + 1/math.Log2(5)) /
				(1 + 1/math.Log2(3) + 1/math.Log2(4)),
		},
	}

	for _, test := range tests {
		t.Run(string(test.metric), func(t *testing.T) {
			threshold := evaluation.Score(0.6)
			evaluator, err := evaluation.NewRetrievalEvaluator(evaluation.RetrievalEvaluatorConfig{
				Metric: test.metric, Cutoff: 4, Threshold: &threshold,
			})
			if err != nil {
				t.Fatal(err)
			}
			report, err := evaluator.Evaluate(t.Context(), sample)
			if err != nil {
				t.Fatal(err)
			}
			if math.Abs(report.Score.Float64()-test.want) > 1e-12 {
				t.Fatalf("score = %v, want %v", report.Score, test.want)
			}
			if got, want := string(report.Metric), "retrieval/"+string(test.metric)+"@4"; got != want {
				t.Fatalf("metric = %q, want %q", got, want)
			}
			if report.Passed != (test.want >= threshold.Float64()) {
				t.Fatalf("passed = %v for score %v and threshold %v", report.Passed, report.Score, threshold)
			}
		})
	}
}

func TestRetrievalPrecisionUsesConfiguredCutoff(t *testing.T) {
	evaluator, err := evaluation.NewRetrievalEvaluator(evaluation.RetrievalEvaluatorConfig{
		Metric: evaluation.RetrievalMetricPrecision,
		Cutoff: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	sample, err := evaluation.NewRetrievalSample([]string{"relevant"}, []string{"relevant"})
	if err != nil {
		t.Fatal(err)
	}
	report, err := evaluator.Evaluate(t.Context(), sample)
	if err != nil {
		t.Fatal(err)
	}
	if report.Score != 0.25 {
		t.Fatalf("precision@4 = %v, want 0.25", report.Score)
	}
}

func TestRetrievalSampleOwnsAndValidatesRankings(t *testing.T) {
	retrieved := []string{"a"}
	relevant := []string{"a"}
	sample, err := evaluation.NewRetrievalSample(retrieved, relevant)
	if err != nil {
		t.Fatal(err)
	}
	retrieved[0], relevant[0] = "changed", "changed"
	clone := sample.Clone()
	clone.Retrieved[0] = "clone"
	if sample.Retrieved[0] != "a" || sample.Relevant[0] != "a" {
		t.Fatalf("sample aliases caller or clone: %#v", sample)
	}

	invalid := []evaluation.RetrievalSample{
		{},
		{Retrieved: []string{" "}, Relevant: []string{"a"}},
		{Retrieved: []string{"a", "a"}, Relevant: []string{"a"}},
		{Retrieved: []string{"a"}, Relevant: []string{"a", "a"}},
	}
	for _, candidate := range invalid {
		if err := candidate.Validate(); !errors.Is(err, evaluation.ErrInvalidSample) {
			t.Fatalf("Validate(%#v) error = %v", candidate, err)
		}
	}
}

func TestRetrievalEvaluatorValidatesConfigAndCancellation(t *testing.T) {
	if _, err := evaluation.NewRetrievalEvaluator(evaluation.RetrievalEvaluatorConfig{Cutoff: 1}); !errors.Is(err, evaluation.ErrInvalidEvaluatorConfig) {
		t.Fatalf("missing metric error = %v", err)
	}
	if _, err := evaluation.NewRetrievalEvaluator(evaluation.RetrievalEvaluatorConfig{Metric: evaluation.RetrievalMetricRecall}); !errors.Is(err, evaluation.ErrInvalidEvaluatorConfig) {
		t.Fatalf("invalid cutoff error = %v", err)
	}
	invalidThreshold := evaluation.Score(2)
	if _, err := evaluation.NewRetrievalEvaluator(evaluation.RetrievalEvaluatorConfig{
		Metric: evaluation.RetrievalMetricRecall, Cutoff: 1, Threshold: &invalidThreshold,
	}); !errors.Is(err, evaluation.ErrInvalidEvaluatorConfig) {
		t.Fatalf("invalid threshold error = %v", err)
	}

	evaluator, err := evaluation.NewRetrievalEvaluator(evaluation.RetrievalEvaluatorConfig{
		Metric: evaluation.RetrievalMetricRecall, Cutoff: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := evaluator.Evaluate(ctx, evaluation.RetrievalSample{Relevant: []string{"a"}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
}
