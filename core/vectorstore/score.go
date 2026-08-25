package vectorstore

import (
	"encoding/json"
	"fmt"
	"math"
)

// Score is a provider-neutral similarity value in [0, 1].
type Score float64

// Float64 returns the score as a primitive provider value.
func (s Score) Float64() float64 { return float64(s) }

// Validate verifies the normalized score contract.
func (s Score) Validate() error {
	value := float64(s)
	if math.IsNaN(value) || math.IsInf(value, 0) || value < MinSimilarityScore || value > MaxSimilarityScore {
		return fmt.Errorf("%w: must be finite and in [%.1f, %.1f], got %v",
			ErrInvalidScore, MinSimilarityScore, MaxSimilarityScore, value)
	}
	return nil
}

func (s Score) MarshalJSON() ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	type wireScore Score
	return json.Marshal(wireScore(s))
}

func (s *Score) UnmarshalJSON(data []byte) error {
	if s == nil {
		return fmt.Errorf("%w: nil Score receiver", ErrInvalidScore)
	}
	type wireScore Score
	var decoded wireScore
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("%w: decode score: %w", ErrInvalidScore, err)
	}
	candidate := Score(decoded)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*s = candidate
	return nil
}

// ScoreFromValue clamps a finite provider score to the common range.
// Non-finite input becomes NaN so result validation reports the contract breach.
func ScoreFromValue(value float64) Score {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return Score(math.NaN())
	}
	return Score(min(MaxSimilarityScore, max(MinSimilarityScore, value)))
}

// ScoreFromCosineSimilarity maps cosine similarity from [-1, 1] to [0, 1].
func ScoreFromCosineSimilarity(similarity float64) Score {
	return ScoreFromValue((similarity + 1) / 2)
}

// ScoreFromCosineDistance maps 1-cosine-similarity from [0, 2] to [0, 1].
func ScoreFromCosineDistance(distance float64) Score {
	return ScoreFromValue(1 - distance/2)
}

// ScoreFromDistance maps a non-negative, unbounded distance to (0, 1], where
// zero is an exact match. Tiny negative values caused by floating-point error
// are treated as zero.
func ScoreFromDistance(distance float64) Score {
	if math.IsNaN(distance) || math.IsInf(distance, 0) {
		return Score(math.NaN())
	}
	return Score(1 / (1 + max(0, distance)))
}

// ScoreFromInnerProduct maps an unbounded dot product monotonically into
// (0, 1).
func ScoreFromInnerProduct(product float64) Score {
	if math.IsNaN(product) || math.IsInf(product, 0) {
		return Score(math.NaN())
	}
	if product >= 0 {
		exponential := math.Exp(-product)
		return Score(1 / (1 + exponential))
	}
	exponential := math.Exp(product)
	return Score(exponential / (1 + exponential))
}

// ScoreFromNegativeInnerProductDistance maps a provider distance defined as
// the negative dot product into the similarity range.
func ScoreFromNegativeInnerProductDistance(distance float64) Score {
	return ScoreFromInnerProduct(-distance)
}

// ScoreFromOneMinusInnerProductDistance maps a provider distance defined as
// 1-dot-product into the similarity range.
func ScoreFromOneMinusInnerProductDistance(distance float64) Score {
	return ScoreFromInnerProduct(1 - distance)
}
