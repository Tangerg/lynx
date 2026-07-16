package turn_test

import (
	"errors"
	"math"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	corechat "github.com/Tangerg/lynx/core/chat"
)

func TestStartTurnRequestValidateDelegatesCoreOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		options corechat.Options
	}{
		{name: "temperature above maximum", options: corechat.Options{Temperature: turnTestPointer(2.1)}},
		{name: "frequency penalty", options: corechat.Options{FrequencyPenalty: turnTestPointer(2.1)}},
		{name: "presence penalty", options: corechat.Options{PresencePenalty: turnTestPointer(-2.1)}},
		{name: "top k", options: corechat.Options{TopK: turnTestPointer(int64(0))}},
		{name: "nan temperature", options: corechat.Options{Temperature: turnTestPointer(math.NaN())}},
		{name: "infinite top p", options: corechat.Options{TopP: turnTestPointer(math.Inf(1))}},
		{name: "negative infinite presence penalty", options: corechat.Options{PresencePenalty: turnTestPointer(math.Inf(-1))}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := (turn.StartTurnRequest{
				SessionID: "session",
				Message:   "hello",
				Options:   &test.options,
			}).Validate()
			if !errors.Is(err, turn.ErrInvalidTurnOptions) {
				t.Fatalf("Validate() error = %v, want ErrInvalidTurnOptions", err)
			}
			if !errors.Is(err, corechat.ErrInvalidOptions) {
				t.Fatalf("Validate() error = %v, want wrapped chat.ErrInvalidOptions", err)
			}
		})
	}
}

func turnTestPointer[T any](value T) *T {
	return &value
}
