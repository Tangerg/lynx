package inmemory

import (
	"math"

	"github.com/Tangerg/scope/core/vectorstore"
)

// Similarity scores two equal-length vectors; higher means more
// similar. Implementations must be deterministic and symmetric:
// Similarity(a, b) == Similarity(b, a). Returning [vectorstore.Score] keeps
// custom strategies inside the same normalized contract as every provider.
type Similarity func(left, right []float64) vectorstore.Score

// CosineSimilarity is the default for [StoreConfig.Similarity] —
// cos(θ) mapped into [0, 1] via (1 + cos) / 2. Returns 0.5 (the
// "no information" midpoint) when either vector has zero magnitude
// rather than NaN.
func CosineSimilarity(left, right []float64) vectorstore.Score {
	if len(left) != len(right) || len(left) == 0 {
		return 0
	}
	var dot, magA, magB float64
	for index := range left {
		dot += left[index] * right[index]
		magA += left[index] * left[index]
		magB += right[index] * right[index]
	}
	if magA == 0 || magB == 0 {
		return vectorstore.ScoreFromCosineSimilarity(0)
	}
	return vectorstore.ScoreFromCosineSimilarity(dot / (math.Sqrt(magA) * math.Sqrt(magB)))
}

// DotProductSimilarity maps the unbounded inner product monotonically into
// the common score range. It is cheaper than [CosineSimilarity] when vector
// magnitude is meaningful or embeddings are already normalized.
func DotProductSimilarity(left, right []float64) vectorstore.Score {
	if len(left) != len(right) {
		return 0
	}
	var dot float64
	for index := range left {
		dot += left[index] * right[index]
	}
	return vectorstore.ScoreFromInnerProduct(dot)
}

// EuclideanSimilarity maps Euclidean distance into [0, 1] via
// 1 / (1 + d). Useful when the embedding space is *not* angular and
// magnitude differences carry information.
func EuclideanSimilarity(left, right []float64) vectorstore.Score {
	if len(left) != len(right) {
		return 0
	}
	var sum float64
	for index := range left {
		difference := left[index] - right[index]
		sum += difference * difference
	}
	return vectorstore.ScoreFromDistance(math.Sqrt(sum))
}
