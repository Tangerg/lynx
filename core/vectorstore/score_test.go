package vectorstore_test

import (
	"math"
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore"
)

func TestScoreNormalization(t *testing.T) {
	tests := []struct {
		name string
		got  float64
		want float64
	}{
		{"opposite similarity", vectorstore.NormalizeCosineSimilarity(-1), 0},
		{"orthogonal similarity", vectorstore.NormalizeCosineSimilarity(0), 0.5},
		{"identical similarity", vectorstore.NormalizeCosineSimilarity(1), 1},
		{"identical distance", vectorstore.NormalizeCosineDistance(0), 1},
		{"orthogonal distance", vectorstore.NormalizeCosineDistance(1), 0.5},
		{"opposite distance", vectorstore.NormalizeCosineDistance(2), 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("score = %v, want %v", test.got, test.want)
			}
		})
	}
}

func TestDistanceNormalizationIsMonotonic(t *testing.T) {
	if got := vectorstore.NormalizeDistance(0); got != 1 {
		t.Fatalf("exact-match score = %v, want 1", got)
	}
	if near, far := vectorstore.NormalizeDistance(1), vectorstore.NormalizeDistance(2); near <= far {
		t.Fatalf("near score %v must exceed far score %v", near, far)
	}
}

func TestInnerProductNormalizationIsStableAndMonotonic(t *testing.T) {
	negative := vectorstore.NormalizeInnerProduct(-1000)
	zero := vectorstore.NormalizeInnerProduct(0)
	positive := vectorstore.NormalizeInnerProduct(1000)
	if !(negative < zero && zero < positive) {
		t.Fatalf("scores are not monotonic: %v, %v, %v", negative, zero, positive)
	}
	if zero != 0.5 || negative != 0 || positive != 1 {
		t.Fatalf("unexpected scores: %v, %v, %v", negative, zero, positive)
	}
}

func TestNonFiniteScoresRemainInvalid(t *testing.T) {
	for _, score := range []float64{
		vectorstore.NormalizeScore(math.Inf(1)),
		vectorstore.NormalizeDistance(math.Inf(1)),
		vectorstore.NormalizeInnerProduct(math.Inf(1)),
	} {
		if !math.IsNaN(score) {
			t.Fatalf("score = %v, want NaN", score)
		}
	}
}
