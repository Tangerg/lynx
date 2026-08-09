package run

import (
	"errors"
	"math"
)

// Limits is the accumulated allowance a Run may consume before it is stopped.
// A zero field is that dimension uncapped, so the zero value is an unbounded Run.
//
// It lives beside [State] and [Outcome] rather than with the accrued
// accounting because it is Run policy — an input admission fixes,
// which the executor enforces and restart recovery must reapply — while
// what was actually spent is a recorded fact.
type Limits struct {
	MaxTotalTokens int64
	MaxSteps       int
	MaxBudgetUSD   float64
}

// Validate reports whether the allowance is expressible. A negative cap is not
// "no cap" — it is a cap nothing can satisfy, and admitting one would stop the
// Run before its first step.
func (l Limits) Validate() error {
	switch {
	case l.MaxTotalTokens < 0:
		return errors.New("run: max total tokens must not be negative")
	case l.MaxSteps < 0:
		return errors.New("run: max steps must not be negative")
	case math.IsNaN(l.MaxBudgetUSD) || math.IsInf(l.MaxBudgetUSD, 0):
		return errors.New("run: max budget USD must be finite")
	case l.MaxBudgetUSD < 0:
		return errors.New("run: max budget USD must not be negative")
	}
	return nil
}

// IsZero reports whether no allowance is in force at all.
func (l Limits) IsZero() bool { return l == Limits{} }
