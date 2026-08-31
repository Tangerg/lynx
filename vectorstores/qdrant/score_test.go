package qdrant

import (
	"math"
	"testing"

	qdrantclient "github.com/qdrant/go-client/qdrant"
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
			if got := test.metric.score(test.raw); math.Abs(got.Float64()-test.want) > 1e-12 {
				t.Fatalf("score(%v) = %v, want %v", test.raw, got, test.want)
			}
		})
	}
}

func TestRawScoreThresholdRoundTrips(t *testing.T) {
	t.Parallel()

	for _, metric := range []DistanceMetric{DistanceCosine, DistanceDot, DistanceEuclid, DistanceManhattan} {
		t.Run(string(metric), func(t *testing.T) {
			t.Parallel()
			raw, ok := metric.rawScoreThreshold(0.75)
			if !ok {
				t.Fatal("rawScoreThreshold() ok = false, want true")
			}
			if got := metric.score(raw); math.Abs(got.Float64()-0.75) > 1e-12 {
				t.Fatalf("threshold round trip = %v, want 0.75", got)
			}
		})
	}
}

func TestZeroScoreOmitsProviderThreshold(t *testing.T) {
	t.Parallel()

	if _, ok := DistanceCosine.rawScoreThreshold(0); ok {
		t.Fatal("rawScoreThreshold(0) ok = true, want false")
	}
}

func TestDistanceMetricIdentityAndProviderMapping(t *testing.T) {
	tests := map[DistanceMetric]qdrantclient.Distance{
		DistanceCosine:    qdrantclient.Distance_Cosine,
		DistanceDot:       qdrantclient.Distance_Dot,
		DistanceEuclid:    qdrantclient.Distance_Euclid,
		DistanceManhattan: qdrantclient.Distance_Manhattan,
	}
	for metric, want := range tests {
		if !metric.Valid() || metric.String() == "" {
			t.Fatalf("metric %q is not self-describing", metric)
		}
		got, err := metric.qdrant()
		if err != nil || got != want {
			t.Fatalf("%s qdrant() = %v, %v, want %v", metric, got, err, want)
		}
	}
	invalid := DistanceMetric("invalid")
	if invalid.Valid() {
		t.Fatal("Valid accepted an unknown metric")
	}
	if _, err := invalid.qdrant(); err == nil {
		t.Fatal("qdrant accepted an unknown metric")
	}
}
