package runs

import (
	"errors"
	"math"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
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

func TestStartTurnValidateRejectsNonCanonicalAdmissionPolicy(t *testing.T) {
	t.Parallel()

	for name, turn := range map[string]StartTurn{
		"negative token limit": {
			Message: "hello", Limits: execution.RunLimits{MaxTotalTokens: -1},
		},
		"non-finite budget": {
			Message: "hello", Limits: execution.RunLimits{MaxBudgetUSD: math.Inf(1)},
		},
		"duplicate interrupt kind": {
			Message: "hello",
			InterruptKinds: []execution.InterruptKind{
				execution.ApprovalInterrupt,
				execution.ApprovalInterrupt,
			},
		},
		"goal lease whitespace": {
			Message: "hello", GoalLeaseID: " lease",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := turn.Validate(); err == nil {
				t.Fatal("Validate accepted non-canonical admission policy")
			}
		})
	}
}

func testPointer[T any](value T) *T {
	return &value
}
