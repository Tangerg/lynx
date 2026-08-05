// Package score contains the one trust boundary for planner score callbacks.
package score

import (
	"math"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/internal/panicerr"
)

// Evaluate invokes a hosted score function and turns its panic into an error.
// Callers retain ownership of whether the result is a cost or value and attach
// the corresponding action or goal identity.
func Evaluate(function core.ScoreFunc, state core.WorldState) (value float64, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = panicerr.New("score function panicked", recovered)
		}
	}()
	return function(state), nil
}

// Finite reports whether value is neither NaN nor an infinity.
func Finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
