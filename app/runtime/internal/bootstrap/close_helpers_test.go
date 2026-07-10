package bootstrap

import (
	"errors"
	"testing"
)

func TestRunClosersRunsAllAndJoinsErrors(t *testing.T) {
	firstErr := errors.New("first")
	lastErr := errors.New("last")
	var calls []int
	err := runClosers([]func() error{
		func() error { calls = append(calls, 1); return firstErr },
		nil,
		func() error { calls = append(calls, 3); return lastErr },
	})
	if !errors.Is(err, firstErr) || !errors.Is(err, lastErr) {
		t.Fatalf("runClosers err = %v, want both errors", err)
	}
	if len(calls) != 2 || calls[0] != 1 || calls[1] != 3 {
		t.Fatalf("calls = %v, want [1 3]", calls)
	}
}
