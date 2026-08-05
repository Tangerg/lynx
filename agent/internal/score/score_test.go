package score_test

import (
	"errors"
	"math"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/internal/score"
)

func TestEvaluateContainsPanicAndFiniteClassifiesResults(t *testing.T) {
	cause := errors.New("score panic")
	if _, err := score.Evaluate(func(core.WorldState) float64 { panic(cause) }, nil); !errors.Is(err, cause) {
		t.Fatalf("Evaluate error = %v, want cause", err)
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if score.Finite(value) {
			t.Fatalf("Finite(%v) = true", value)
		}
	}
	if !score.Finite(0) {
		t.Fatal("Finite(0) = false")
	}
}
