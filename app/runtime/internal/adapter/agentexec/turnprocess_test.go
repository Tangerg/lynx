package agentexec

import (
	"errors"
	"testing"
)

func TestTurnCompletionErrorPreservesIndependentFailures(t *testing.T) {
	t.Parallel()

	processFailure := errors.New("process failed")
	finalizeFailure := errors.New("snapshot failed")
	completion := TurnCompletion{Failure: processFailure, Err: finalizeFailure}

	if err := completion.Error(); !errors.Is(err, processFailure) || !errors.Is(err, finalizeFailure) {
		t.Fatalf("Error() = %v, want both independent failures", err)
	}
}
