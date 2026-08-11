package runs

import (
	"errors"
	"math"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	corechat "github.com/Tangerg/lynx/core/chat"
)

func TestStartExecutionValidateDelegatesCoreOptions(t *testing.T) {
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

			err := (RootExecutionStart{Message: "hello", Options: &test.options}).Validate()
			if !errors.Is(err, ErrInvalidRunOptions) {
				t.Fatalf("Validate() error = %v, want ErrInvalidRunOptions", err)
			}
			if !errors.Is(err, corechat.ErrInvalidOptions) {
				t.Fatalf("Validate() error = %v, want wrapped chat.ErrInvalidOptions", err)
			}
		})
	}
}

func TestStartExecutionValidateKeepsModelSelectionOutsideOptions(t *testing.T) {
	t.Parallel()

	err := (RootExecutionStart{
		Message: "hello",
		Options: &corechat.Options{
			Model: "model-inside-options",
		},
	}).Validate()
	if !errors.Is(err, ErrInvalidRunOptions) {
		t.Fatalf("Validate() error = %v, want ErrInvalidRunOptions", err)
	}
}

func TestStartExecutionValidateRejectsNonCanonicalAdmissionPolicy(t *testing.T) {
	t.Parallel()

	for name, execution := range map[string]RootExecutionStart{
		"negative token limit": {
			Message: "hello", Limits: run.Limits{MaxTotalTokens: -1},
		},
		"non-finite budget": {
			Message: "hello", Limits: run.Limits{MaxBudgetUSD: math.Inf(1)},
		},
		"duplicate interrupt kind": {
			Message: "hello",
			InterruptKinds: []interrupt.Kind{
				interrupt.Approval,
				interrupt.Approval,
			},
		},
		"goal incarnation whitespace": {
			Message: "hello", GoalIncarnationID: " lease",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := execution.Validate(); err == nil {
				t.Fatal("Validate accepted non-canonical admission policy")
			}
		})
	}
}

func testPointer[T any](value T) *T {
	return &value
}
