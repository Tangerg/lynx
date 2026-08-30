package inmemory_test

import (
	"math"
	"testing"

	"github.com/Tangerg/scope/core/vectorstore"
	"github.com/Tangerg/scope/core/vectorstore/inmemory"
)

// TestSimilarityFunctionsShareTheNormalizedContract keeps every strategy inside
// the same [0, 1] score range providers are held to, so swapping the strategy
// cannot change what MinScore means.
func TestSimilarityFunctionsShareTheNormalizedContract(t *testing.T) {
	strategies := map[string]inmemory.Similarity{
		"cosine":      inmemory.CosineSimilarity,
		"dot product": inmemory.DotProductSimilarity,
		"euclidean":   inmemory.EuclideanSimilarity,
	}
	vectors := [][2][]float64{
		{{1, 0, 0}, {1, 0, 0}},
		{{1, 0, 0}, {0, 1, 0}},
		{{1, 0, 0}, {-1, 0, 0}},
		{{0.5, 0.5}, {0.25, 0.75}},
		{{1000, -1000}, {-1000, 1000}},
	}
	for name, strategy := range strategies {
		t.Run(name, func(t *testing.T) {
			for _, pair := range vectors {
				score := strategy(pair[0], pair[1])
				if err := score.Validate(); err != nil {
					t.Fatalf("%v vs %v scored %v: %v", pair[0], pair[1], score, err)
				}
			}
		})
	}
}

// TestSimilarityIsSymmetric is part of the [inmemory.Similarity] contract: an
// asymmetric strategy would make result ordering depend on map iteration order.
func TestSimilarityIsSymmetric(t *testing.T) {
	left := []float64{0.1, -0.4, 0.9}
	right := []float64{0.7, 0.2, -0.3}
	strategies := map[string]inmemory.Similarity{
		"cosine":      inmemory.CosineSimilarity,
		"dot product": inmemory.DotProductSimilarity,
		"euclidean":   inmemory.EuclideanSimilarity,
	}
	for name, strategy := range strategies {
		t.Run(name, func(t *testing.T) {
			if strategy(left, right) != strategy(right, left) {
				t.Fatalf("%s is not symmetric", name)
			}
		})
	}
}

// TestSimilarityRejectsMismatchedVectors keeps a dimension mismatch from
// producing a partial score: an incomparable pair must score zero, not the
// similarity of its shared prefix.
func TestSimilarityRejectsMismatchedVectors(t *testing.T) {
	strategies := map[string]inmemory.Similarity{
		"cosine":      inmemory.CosineSimilarity,
		"dot product": inmemory.DotProductSimilarity,
		"euclidean":   inmemory.EuclideanSimilarity,
	}
	for name, strategy := range strategies {
		t.Run(name, func(t *testing.T) {
			if score := strategy([]float64{1, 2}, []float64{1}); score != 0 {
				t.Fatalf("%s scored mismatched vectors %v", name, score)
			}
		})
	}
	if score := inmemory.CosineSimilarity(nil, nil); score != 0 {
		t.Fatalf("cosine scored empty vectors %v", score)
	}
}

// TestCosineSimilarityHandlesZeroMagnitude documents why a zero vector scores
// the midpoint rather than NaN: an unscoreable pair must still sort predictably
// instead of poisoning the comparison.
func TestCosineSimilarityHandlesZeroMagnitude(t *testing.T) {
	score := inmemory.CosineSimilarity([]float64{0, 0}, []float64{1, 1})
	if err := score.Validate(); err != nil {
		t.Fatal(err)
	}
	if score != vectorstore.ScoreFromCosineSimilarity(0) {
		t.Fatalf("zero-magnitude score = %v, want the no-information midpoint", score)
	}
	if math.IsNaN(float64(score)) {
		t.Fatal("zero-magnitude vectors produced NaN")
	}
}

// TestSimilarityOrdersByCloseness is the property search depends on: the more
// similar pair must score strictly higher under every strategy.
func TestSimilarityOrdersByCloseness(t *testing.T) {
	query := []float64{1, 0}
	near := []float64{0.9, 0.1}
	far := []float64{-1, 0}
	strategies := map[string]inmemory.Similarity{
		"cosine":      inmemory.CosineSimilarity,
		"dot product": inmemory.DotProductSimilarity,
		"euclidean":   inmemory.EuclideanSimilarity,
	}
	for name, strategy := range strategies {
		t.Run(name, func(t *testing.T) {
			if strategy(query, near) <= strategy(query, far) {
				t.Fatalf("%s did not rank the closer vector higher", name)
			}
		})
	}
}
