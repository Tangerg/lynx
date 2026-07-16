package runs

import (
	"errors"
	"math"
	"testing"

	corechat "github.com/Tangerg/lynx/core/chat"
)

func TestStartTurnValidateDelegatesCoreOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		options corechat.Options
	}{
		{name: "temperature above maximum", options: corechat.Options{Temperature: testPointer(2.1)}},
		{name: "frequency penalty", options: corechat.Options{FrequencyPenalty: testPointer(2.1)}},
		{name: "presence penalty", options: corechat.Options{PresencePenalty: testPointer(-2.1)}},
		{name: "top k", options: corechat.Options{TopK: testPointer(int64(0))}},
		{name: "nan temperature", options: corechat.Options{Temperature: testPointer(math.NaN())}},
		{name: "infinite top p", options: corechat.Options{TopP: testPointer(math.Inf(1))}},
		{name: "negative infinite presence penalty", options: corechat.Options{PresencePenalty: testPointer(math.Inf(-1))}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := (StartTurn{Message: "hello", Options: &test.options}).Validate()
			if !errors.Is(err, ErrInvalidTurnOptions) {
				t.Fatalf("Validate() error = %v, want ErrInvalidTurnOptions", err)
			}
			if !errors.Is(err, corechat.ErrInvalidOptions) {
				t.Fatalf("Validate() error = %v, want wrapped chat.ErrInvalidOptions", err)
			}
		})
	}
}

func TestStartTurnValidateKeepsModelSelectionOutsideOptions(t *testing.T) {
	t.Parallel()

	err := (StartTurn{
		Message: "hello",
		Options: &corechat.Options{
			Model: "model-inside-options",
		},
	}).Validate()
	if !errors.Is(err, ErrInvalidTurnOptions) {
		t.Fatalf("Validate() error = %v, want ErrInvalidTurnOptions", err)
	}
}

func testPointer[T any](value T) *T {
	return &value
}
