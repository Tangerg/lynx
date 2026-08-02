package scores_test

import (
	"math"
	"testing"

	"github.com/Tangerg/lynx/internal/vectorstorekit/scores"
)

func TestCosineConversions(t *testing.T) {
	for _, test := range []struct {
		name string
		got  float64
		want float64
	}{
		{"opposite similarity", scores.CosineSimilarity(-1), 0},
		{"orthogonal similarity", scores.CosineSimilarity(0), 0.5},
		{"identical similarity", scores.CosineSimilarity(1), 1},
		{"identical distance", scores.CosineDistance(0), 1},
		{"orthogonal distance", scores.CosineDistance(1), 0.5},
		{"opposite distance", scores.CosineDistance(2), 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("score = %v, want %v", test.got, test.want)
			}
		})
	}
}

func TestDistanceIsMonotonic(t *testing.T) {
	if got := scores.Distance(0); got != 1 {
		t.Fatalf("exact-match score = %v, want 1", got)
	}
	if near, far := scores.Distance(1), scores.Distance(2); near <= far {
		t.Fatalf("near score %v must exceed far score %v", near, far)
	}
}

func TestInnerProductIsStableAndMonotonic(t *testing.T) {
	negative := scores.InnerProduct(-1000)
	zero := scores.InnerProduct(0)
	positive := scores.InnerProduct(1000)
	if !(negative < zero && zero < positive) {
		t.Fatalf("scores are not monotonic: %v, %v, %v", negative, zero, positive)
	}
	if zero != 0.5 || negative != 0 || positive != 1 {
		t.Fatalf("unexpected scores: %v, %v, %v", negative, zero, positive)
	}
}

func TestNonFiniteInputRemainsInvalid(t *testing.T) {
	for _, score := range []float64{
		scores.Bounded(math.Inf(1)),
		scores.Distance(math.Inf(1)),
		scores.InnerProduct(math.Inf(1)),
	} {
		if !math.IsNaN(score) {
			t.Fatalf("score = %v, want NaN", score)
		}
	}
}
