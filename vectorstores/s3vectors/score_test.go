package s3vectors

import (
	"math"
	"testing"
)

func TestDistanceMetricContract(t *testing.T) {
	tests := []struct {
		metric   DistanceMetric
		distance float64
		want     float64
	}{
		{metric: DistanceCosine, distance: 0, want: 1},
		{metric: DistanceEuclidean, distance: 1, want: 0.5},
	}
	for _, test := range tests {
		if !test.metric.Valid() || test.metric.String() == "" {
			t.Fatalf("metric %q is not self-describing", test.metric)
		}
		if got := test.metric.score(test.distance).Float64(); math.Abs(got-test.want) > 1e-12 {
			t.Fatalf("%s score(%v) = %v, want %v", test.metric, test.distance, got, test.want)
		}
	}
	if DistanceMetric("invalid").Valid() {
		t.Fatal("Valid accepted an unknown metric")
	}
}
