package random

import (
	"math/rand/v2"
)

// IntRange returns a random integer in [min, max) range.
// Parameters:
//   - min: the lower bound of the range
//   - max: the upper bound of the range
//
// It panics if min > max.
// It panics if max <=0. (by standard rand)
// Returns a random integer in [min, max) range.
func IntRange(min, max int) int {
	if min > max {
		panic("IntRange: min cannot be greater than max")
	}
	return rand.IntN(max-min) + min
}
