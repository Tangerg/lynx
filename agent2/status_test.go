package agent2

import (
	"errors"
	"testing"
)

func TestStatusTransitionMatrix(t *testing.T) {
	legal := map[Status]map[Status]bool{
		StatusNotStarted: {StatusRunning: true},
		StatusRunning: {
			StatusWaiting: true, StatusPaused: true, StatusCompleted: true, StatusFailed: true,
			StatusCancelled: true, StatusTimedOut: true, StatusKilled: true,
		},
		StatusWaiting: {
			StatusRunning: true, StatusFailed: true, StatusCancelled: true, StatusTimedOut: true, StatusKilled: true,
		},
		StatusPaused: {
			StatusRunning: true, StatusCancelled: true, StatusTimedOut: true, StatusKilled: true,
		},
	}
	statuses := []Status{
		StatusNotStarted, StatusRunning, StatusWaiting, StatusPaused, StatusCompleted,
		StatusFailed, StatusCancelled, StatusTimedOut, StatusKilled,
	}
	for _, from := range statuses {
		for _, to := range statuses {
			if got, want := from.CanTransitionTo(to), legal[from][to]; got != want {
				t.Errorf("%s.CanTransitionTo(%s) = %t, want %t", from, to, got, want)
			}
		}
	}
}

func TestResolveTerminationPriorityMatrix(t *testing.T) {
	kill, _ := newKillIntent("operator requested kill")
	deadline, _ := newDeadlineIntent(deadlineOwnerHost, "host deadline reached")
	cancellation, _ := newCancellationIntent(cancellationOwnerParent, "parent cancelled")
	failure, _ := NewFailure(FailureKindExternal, "provider.failed", "Provider failed.")
	failed, _ := failedOutcome(failure)

	tests := []struct {
		name  string
		facts terminationFacts
		want  Status
		cause TerminationCause
	}{
		{name: "kill wins all", facts: terminationFacts{kill: kill, deadline: deadline, cancellation: cancellation, outcome: failed}, want: StatusKilled, cause: TerminationCauseEngineKill},
		{name: "deadline wins cancellation and failure", facts: terminationFacts{deadline: deadline, cancellation: cancellation, outcome: failed}, want: StatusTimedOut, cause: TerminationCauseHostDeadline},
		{name: "cancellation wins failure", facts: terminationFacts{cancellation: cancellation, outcome: failed}, want: StatusCancelled, cause: TerminationCauseParentCancellation},
		{name: "failure without control", facts: terminationFacts{outcome: failed}, want: StatusFailed, cause: TerminationCauseExternalFailure},
		{name: "completion", facts: terminationFacts{outcome: completedOutcome()}, want: StatusCompleted, cause: TerminationCauseCompletion},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := resolveTermination(test.facts)
			if err != nil {
				t.Fatal(err)
			}
			if !got.Valid() || got.Status() != test.want || got.Cause() != test.cause {
				t.Fatalf("ResolveTermination() = status %s cause %d valid %t", got.Status(), got.Cause(), got.Valid())
			}
		})
	}
}

func TestResolveTerminationRejectsMissingFacts(t *testing.T) {
	if _, err := resolveTermination(terminationFacts{}); !errors.Is(err, errInvalidTermination) {
		t.Fatalf("resolveTermination(empty) error = %v, want errInvalidTermination", err)
	}
	if _, err := newDeadlineIntent(deadlineOwnerInvalid, "deadline"); !errors.Is(err, errInvalidTermination) {
		t.Fatalf("newDeadlineIntent error = %v, want errInvalidTermination", err)
	}
}
