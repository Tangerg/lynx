package etl_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/scope/etl"
)

func TestSourceBudgetRejectsOversizedInputWithoutReturningPartialData(t *testing.T) {
	budget, err := etl.NewSourceBudget(4)
	if err != nil {
		t.Fatal(err)
	}
	data, err := budget.ReadAll(t.Context(), strings.NewReader("12345"))
	if data != nil || !errors.Is(err, etl.ErrSourceTooLarge) {
		t.Fatalf("ReadAll = (%q, %v), want nil ErrSourceTooLarge", data, err)
	}
}

func TestSourceBudgetAcceptsExactBoundary(t *testing.T) {
	budget, err := etl.NewSourceBudget(4)
	if err != nil {
		t.Fatal(err)
	}
	data, err := budget.ReadAll(t.Context(), strings.NewReader("1234"))
	if err != nil || string(data) != "1234" {
		t.Fatalf("ReadAll = (%q, %v)", data, err)
	}
}

func TestSourceBudgetZeroValueUsesBoundedDefault(t *testing.T) {
	var budget etl.SourceBudget
	if budget.MaxBytes() != etl.DefaultMaxSourceBytes {
		t.Fatalf("MaxBytes = %d, want %d", budget.MaxBytes(), etl.DefaultMaxSourceBytes)
	}
}

func TestNewSourceBudgetRejectsNonPositiveValues(t *testing.T) {
	for _, maxBytes := range []int64{-1, 0} {
		if _, err := etl.NewSourceBudget(maxBytes); !errors.Is(err, etl.ErrInvalidSourceBudget) {
			t.Fatalf("NewSourceBudget(%d) error = %v", maxBytes, err)
		}
	}
}

func TestSourceBudgetHonorsCanceledContextBeforeReading(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	data, err := (etl.SourceBudget{}).ReadAll(ctx, strings.NewReader("unread"))
	if data != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("ReadAll = (%q, %v), want nil context.Canceled", data, err)
	}
}
