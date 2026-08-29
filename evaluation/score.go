package evaluation

import (
	"fmt"
	"math"
)

// Score is a normalized quality score in the closed interval [0, 1], where a
// higher value is always better.
type Score float64

const DefaultThreshold Score = 0.5

func NewScore(value float64) (Score, error) {
	score := Score(value)
	if err := score.Validate(); err != nil {
		return 0, err
	}
	return score, nil
}

func (score Score) Float64() float64 { return float64(score) }

func (score Score) Validate() error {
	value := score.Float64()
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
		return fmt.Errorf("%w: must be between 0 and 1", ErrInvalidScore)
	}
	return nil
}

// Verdict returns the categorical judgment for a valid threshold.
func (score Score) Verdict(threshold Score) (Verdict, error) {
	if err := score.Validate(); err != nil {
		return VerdictUnspecified, err
	}
	if err := threshold.Validate(); err != nil {
		return VerdictUnspecified, fmt.Errorf("evaluation: threshold: %w", err)
	}
	if score >= threshold {
		return VerdictPass, nil
	}
	return VerdictFail, nil
}
