package qdrant

import (
	"math"
	"testing"
)

func TestScoreNormalization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		metric DistanceMetric
		raw    float64
		want   float64
	}{
		{name: "cosine opposite", metric: DistanceCosine, raw: -1, want: 0},
		{name: "cosine equal", metric: DistanceCosine, raw: 1, want: 1},
		{name: "dot zero", metric: DistanceDot, raw: 0, want: 0.5},
		{name: "euclid equal", metric: DistanceEuclid, raw: 0, want: 1},
		{name: "manhattan distance", metric: DistanceManhattan, raw: 3, want: 0.25},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			store := Store{distanceMetric: test.metric}
			if got := store.normalizeScore(test.raw); math.Abs(got-test.want) > 1e-12 {
				t.Fatalf("normalizeScore(%v) = %v, want %v", test.raw, got, test.want)
			}
		})
	}
}

func TestRawScoreThresholdRoundTrips(t *testing.T) {
	t.Parallel()

	for _, metric := range []DistanceMetric{DistanceCosine, DistanceDot, DistanceEuclid, DistanceManhattan} {
		t.Run(string(metric), func(t *testing.T) {
			t.Parallel()
			store := Store{distanceMetric: metric}
			raw, ok := store.rawScoreThreshold(0.75)
			if !ok {
				t.Fatal("rawScoreThreshold() ok = false, want true")
			}
			if got := store.normalizeScore(raw); math.Abs(got-0.75) > 1e-12 {
				t.Fatalf("threshold round trip = %v, want 0.75", got)
			}
		})
	}
}

func TestZeroScoreOmitsProviderThreshold(t *testing.T) {
	t.Parallel()

	if _, ok := (&Store{distanceMetric: DistanceCosine}).rawScoreThreshold(0); ok {
		t.Fatal("rawScoreThreshold(0) ok = true, want false")
	}
}
