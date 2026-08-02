// Package scores converts provider-specific vector distances and
// similarities into Lynx's finite [0, 1] similarity-score contract.
package scores

import "math"

// Bounded clamps a finite provider score to [0, 1]. Non-finite input becomes
// NaN so the vectorstore result validator reports a provider contract breach.
func Bounded(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return math.NaN()
	}
	switch {
	case value < 0:
		return 0
	case value > 1:
		return 1
	default:
		return value
	}
}

// CosineSimilarity maps cosine similarity from [-1, 1] to [0, 1].
func CosineSimilarity(similarity float64) float64 {
	return Bounded((similarity + 1) / 2)
}

// CosineDistance maps 1-cosine-similarity from [0, 2] to [0, 1].
func CosineDistance(distance float64) float64 {
	return Bounded(1 - distance/2)
}

// Distance maps a non-negative, unbounded distance to (0, 1], where zero is
// an exact match. Tiny negative values caused by floating-point error are
// treated as zero.
func Distance(distance float64) float64 {
	if math.IsNaN(distance) || math.IsInf(distance, 0) {
		return math.NaN()
	}
	return 1 / (1 + max(0, distance))
}

// InnerProduct maps an unbounded dot product monotonically into (0, 1).
func InnerProduct(product float64) float64 {
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

// NegativeInnerProductDistance maps a provider distance defined as the
// negative dot product into Lynx's similarity range.
func NegativeInnerProductDistance(distance float64) float64 {
	return InnerProduct(-distance)
}

// OneMinusInnerProductDistance maps a provider distance defined as
// 1-dot-product into Lynx's similarity range.
func OneMinusInnerProductDistance(distance float64) float64 {
	return InnerProduct(1 - distance)
}
