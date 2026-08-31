package azurecosmos

import (
	"math"
	"testing"
)

func TestNormalizeScore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		function DistanceFunction
		raw      float64
		want     float64
	}{
		{name: "cosine opposite", function: DistanceCosine, raw: -1, want: 0},
		{name: "cosine equal", function: DistanceCosine, raw: 1, want: 1},
		{name: "dot zero", function: DistanceDotProduct, raw: 0, want: 0.5},
		{name: "euclidean equal", function: DistanceEuclidean, raw: 0, want: 1},
		{name: "euclidean distance", function: DistanceEuclidean, raw: 3, want: 0.25},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := test.function.score(test.raw); math.Abs(got.Float64()-test.want) > 1e-12 {
				t.Fatalf("score(%v) = %v, want %v", test.raw, got, test.want)
			}
		})
	}
}

func TestDistanceFunctionIdentity(t *testing.T) {
	for _, function := range []DistanceFunction{DistanceCosine, DistanceDotProduct, DistanceEuclidean} {
		if !function.Valid() || function.String() == "" {
			t.Fatalf("distance function %q is not self-describing", function)
		}
	}
	if DistanceFunction("invalid").Valid() {
		t.Fatal("Valid accepted an unknown distance function")
	}
}
