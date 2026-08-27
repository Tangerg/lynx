package evaluation

import (
	"fmt"
	"math"
)

// Score is a normalized evaluation score in the closed interval [0, 1].
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

// Passes rejects an invalid score or threshold instead of allowing NaN
// comparison semantics to leak into verdicts.
func (score Score) Passes(threshold Score) bool {
	return score.Validate() == nil && threshold.Validate() == nil && score >= threshold
}

func (score *Score) valueOr(fallback Score) (Score, error) {
	if score == nil {
		return fallback, nil
	}
	if err := score.Validate(); err != nil {
		return 0, err
	}
	return *score, nil
}
