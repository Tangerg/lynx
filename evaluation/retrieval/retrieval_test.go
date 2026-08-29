package retrieval_test

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/Tangerg/scope/evaluation"
	"github.com/Tangerg/scope/evaluation/retrieval"
)

func TestEvaluatorCalculatesRankingMetricsAtCutoff(t *testing.T) {
	sample, err := retrieval.NewSample(
		[]string{"a", "b", "c", "d"},
		[]string{"b", "d", "e"},
	)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		metric retrieval.Metric
		want   float64
	}{
		{metric: retrieval.MetricPrecision, want: 0.5},
		{metric: retrieval.MetricRecall, want: 2.0 / 3.0},
		{metric: retrieval.MetricReciprocalRank, want: 0.5},
		{
			metric: retrieval.MetricNDCG,
			want: (1/math.Log2(3) + 1/math.Log2(5)) /
				(1 + 1/math.Log2(3) + 1/math.Log2(4)),
		},
	}

	for _, test := range tests {
		t.Run(string(test.metric), func(t *testing.T) {
			threshold := evaluation.Score(0.6)
			evaluator, err := retrieval.NewEvaluator(retrieval.Config{
				Metric: test.metric, Cutoff: 4, Threshold: &threshold,
			})
			if err != nil {
				t.Fatal(err)
			}
			report, err := evaluator.Evaluate(t.Context(), sample)
			if err != nil {
				t.Fatal(err)
			}
			if report.Score == nil || math.Abs(report.Score.Float64()-test.want) > 1e-12 {
				t.Fatalf("score = %v, want %v", report.Score, test.want)
			}
			if report.Metric.Namespace != "retrieval" || report.Metric.Name != evaluation.MetricName(test.metric) {
				t.Fatalf("metric = %#v", report.Metric)
			}
			cutoff, found, err := report.Metric.Parameters.Decode[int]("cutoff")
			if err != nil || !found || cutoff != 4 {
				t.Fatalf("metric cutoff = (%d, %v, %v)", cutoff, found, err)
			}
			wantVerdict := evaluation.VerdictFail
			if test.want >= threshold.Float64() {
				wantVerdict = evaluation.VerdictPass
			}
			if report.Verdict != wantVerdict {
				t.Fatalf("verdict = %v for score %v and threshold %v", report.Verdict, report.Score, threshold)
			}
		})
	}
}

func TestPrecisionUsesConfiguredCutoff(t *testing.T) {
	evaluator, err := retrieval.NewEvaluator(retrieval.Config{
		Metric: retrieval.MetricPrecision,
		Cutoff: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	sample, err := retrieval.NewSample([]string{"relevant"}, []string{"relevant"})
	if err != nil {
		t.Fatal(err)
	}
	report, err := evaluator.Evaluate(t.Context(), sample)
	if err != nil {
		t.Fatal(err)
	}
	if report.Score == nil || *report.Score != 0.25 {
		t.Fatalf("precision@4 = %v, want 0.25", report.Score)
	}
}

func TestSampleOwnsAndValidatesRankings(t *testing.T) {
	retrieved := []string{"a"}
	relevant := []string{"a"}
	sample, err := retrieval.NewSample(retrieved, relevant)
	if err != nil {
		t.Fatal(err)
	}
	retrieved[0], relevant[0] = "changed", "changed"
	clone := sample.Clone()
	clone.Retrieved[0] = "clone"
	if sample.Retrieved[0] != "a" || sample.Relevant[0] != "a" {
		t.Fatalf("sample aliases caller or clone: %#v", sample)
	}

	invalid := []retrieval.Sample{
		{},
		{Retrieved: []string{" "}, Relevant: []string{"a"}},
		{Retrieved: []string{"a", "a"}, Relevant: []string{"a"}},
		{Retrieved: []string{"a"}, Relevant: []string{"a", "a"}},
	}
	for _, candidate := range invalid {
		if err := candidate.Validate(); !errors.Is(err, retrieval.ErrInvalidSample) {
			t.Fatalf("Validate(%#v) error = %v", candidate, err)
		}
	}
}

func TestEvaluatorValidatesConfigAndCancellation(t *testing.T) {
	if _, err := retrieval.NewEvaluator(retrieval.Config{Cutoff: 1}); !errors.Is(err, evaluation.ErrInvalidEvaluatorConfig) {
		t.Fatalf("missing metric error = %v", err)
	}
	if _, err := retrieval.NewEvaluator(retrieval.Config{Metric: retrieval.MetricRecall}); !errors.Is(err, evaluation.ErrInvalidEvaluatorConfig) {
		t.Fatalf("invalid cutoff error = %v", err)
	}
	invalidThreshold := evaluation.Score(2)
	if _, err := retrieval.NewEvaluator(retrieval.Config{
		Metric: retrieval.MetricRecall, Cutoff: 1, Threshold: &invalidThreshold,
	}); !errors.Is(err, evaluation.ErrInvalidEvaluatorConfig) {
		t.Fatalf("invalid threshold error = %v", err)
	}

	evaluator, err := retrieval.NewEvaluator(retrieval.Config{
		Metric: retrieval.MetricRecall, Cutoff: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := evaluator.Evaluate(ctx, retrieval.Sample{Relevant: []string{"a"}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
}
