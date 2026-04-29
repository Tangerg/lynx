package random

import (
	"math/rand/v2"
)

// IntRange returns a pseudo-random int in the half-open interval [lo, hi).
// It panics if lo >= hi.
//
// Example:
//
//	n := random.IntRange(1, 7) // a value in [1, 7), i.e. a six-sided die roll
func IntRange(lo, hi int) int {
	if lo >= hi {
		panic("random: lo must be less than hi")
	}
	return rand.IntN(hi-lo) + lo
}
