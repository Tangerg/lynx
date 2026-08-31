package azureaisearch

import (
	"math"
	"testing"
)

func TestSimilarityMetricContract(t *testing.T) {
	tests := []struct {
		metric SimilarityMetric
		raw    float64
		want   float64
	}{
		{metric: SimilarityCosine, raw: 1, want: 1},
		{metric: SimilarityDot, raw: 0.5, want: 0.5},
		{metric: SimilarityEuclidean, raw: 0.25, want: 0.25},
	}
	for _, test := range tests {
		if !test.metric.Valid() || test.metric.String() == "" {
			t.Fatalf("metric %q is not self-describing", test.metric)
		}
		if got := test.metric.score(test.raw).Float64(); math.Abs(got-test.want) > 1e-12 {
			t.Fatalf("%s score(%v) = %v, want %v", test.metric, test.raw, got, test.want)
		}
	}
	if SimilarityMetric("invalid").Valid() {
		t.Fatal("Valid accepted an unknown metric")
	}
}
