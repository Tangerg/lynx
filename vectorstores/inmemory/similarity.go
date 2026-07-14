package inmemory

import "math"

// Similarity scores two equal-length vectors; higher means more
// similar. Implementations must be deterministic and symmetric:
// Similarity(a, b) == Similarity(b, a). The returned score is passed
// straight through to [vectorstore.SearchRequest.MinScore]; callers
// should pick a threshold appropriate to the function they choose.
type Similarity func(a, b []float64) float64

// CosineSimilarity is the default for [StoreConfig.Similarity] —
// cos(θ) mapped into [0, 1] via (1 + cos) / 2. Returns 0.5 (the
// "no information" midpoint) when either vector has zero magnitude
// rather than NaN.
func CosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, magA, magB float64
	for i := range a {
		dot += a[i] * b[i]
		magA += a[i] * a[i]
		magB += b[i] * b[i]
	}
	if magA == 0 || magB == 0 {
		return 0.5
	}
	return (1 + dot/(math.Sqrt(magA)*math.Sqrt(magB))) / 2
}

// DotProductSimilarity returns the raw inner product without
// normalisation. Suitable when the embedding model already produces
// unit-length vectors; cheaper than [CosineSimilarity]. Result is
// **not** clipped to [0, 1] — callers using this with
// [vectorstore.SearchRequest.MinScore] should set their threshold
// accordingly.
func DotProductSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot float64
	for i := range a {
		dot += a[i] * b[i]
	}
	return dot
}

// EuclideanSimilarity maps Euclidean distance into [0, 1] via
// 1 / (1 + d). Useful when the embedding space is *not* angular and
// magnitude differences carry information.
func EuclideanSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}
	var sum float64
	for i := range a {
		d := a[i] - b[i]
		sum += d * d
	}
	return 1 / (1 + math.Sqrt(sum))
}
