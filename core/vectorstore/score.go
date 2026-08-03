package vectorstore

import "math"

// NormalizeScore clamps a finite provider score to [MinSimilarityScore,
// MaxSimilarityScore]. Non-finite input becomes NaN so result validation can
// report the provider contract breach.
func NormalizeScore(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return math.NaN()
	}
	return min(MaxSimilarityScore, max(MinSimilarityScore, value))
}

// NormalizeCosineSimilarity maps cosine similarity from [-1, 1] to [0, 1].
func NormalizeCosineSimilarity(similarity float64) float64 {
	return NormalizeScore((similarity + 1) / 2)
}

// NormalizeCosineDistance maps 1-cosine-similarity from [0, 2] to [0, 1].
func NormalizeCosineDistance(distance float64) float64 {
	return NormalizeScore(1 - distance/2)
}

// NormalizeDistance maps a non-negative, unbounded distance to (0, 1], where
// zero is an exact match. Tiny negative values caused by floating-point error
// are treated as zero.
func NormalizeDistance(distance float64) float64 {
	if math.IsNaN(distance) || math.IsInf(distance, 0) {
		return math.NaN()
	}
	return 1 / (1 + max(0, distance))
}

// NormalizeInnerProduct maps an unbounded dot product monotonically into
// (0, 1).
func NormalizeInnerProduct(product float64) float64 {
	if math.IsNaN(product) || math.IsInf(product, 0) {
		return math.NaN()
	}
	if product >= 0 {
		exponential := math.Exp(-product)
		return 1 / (1 + exponential)
	}
	exponential := math.Exp(product)
	return exponential / (1 + exponential)
}

// NormalizeNegativeInnerProductDistance maps a provider distance defined as
// the negative dot product into the similarity range.
func NormalizeNegativeInnerProductDistance(distance float64) float64 {
	return NormalizeInnerProduct(-distance)
}

// NormalizeOneMinusInnerProductDistance maps a provider distance defined as
// 1-dot-product into the similarity range.
func NormalizeOneMinusInnerProductDistance(distance float64) float64 {
	return NormalizeInnerProduct(1 - distance)
}
