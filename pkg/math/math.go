package math

import (
	"errors"
	"math"
)

// Abs returns the absolute value of an integer.
// It supports different integer types using generics.
func Abs[T int | int8 | int16 | int32 | int64](x T) T {
	if x < 0 {
		return -x
	}
	return x
}

// MultiplyExact multiplies two int64 numbers and checks for overflow.
// Returns the result of multiplication and an error if overflow occurs.
func MultiplyExact(x, y int64) (int64, error) {
	r := x * y
	ax := Abs(x)
	ay := Abs(y)
	if ax|ay>>31 != 0 { // Check if either operand is large enough to potentially cause overflow
		// If y isn't zero and the result divided by y is not equal to x (overflow check)
		// Also check special case of MinInt64 * -1 overflow
		if (y != 0 && r/y != x) || (x == math.MinInt64 && y == -1) {
			return 0, errors.New("int64 overflow")
		}
	}
	return r, nil
}

// DivideExact divides x by y and checks for overflow.
// Returns the result of division and an error if overflow occurs.
func DivideExact(x int64, y int64) (int64, error) {
	q := x / y
	// Check for overflow using bitwise operations on x, y, and the quotient
	if (x & y & q) >= 0 {
		return q, nil // Return the result if no overflow
	}
	return 0, errors.New("int64 overflow")
}
