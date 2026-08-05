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
	kill, _ := NewKillIntent("operator requested kill")
	deadline, _ := NewDeadlineIntent(DeadlineOwnerHost, "host deadline reached")
	cancellation, _ := NewCancellationIntent(CancellationOwnerParent, "parent cancelled")
	failure, _ := NewFailure(FailureKindExternal, "provider.failed", "Provider failed.")
	failed, _ := FailedOutcome(failure)

	tests := []struct {
		name  string
		facts TerminationFacts
		want  Status
		cause TerminationCause
	}{
		{name: "kill wins all", facts: TerminationFacts{Kill: kill, Deadline: deadline, Cancellation: cancellation, Outcome: failed}, want: StatusKilled, cause: TerminationCauseEngineKill},
		{name: "deadline wins cancellation and failure", facts: TerminationFacts{Deadline: deadline, Cancellation: cancellation, Outcome: failed}, want: StatusTimedOut, cause: TerminationCauseHostDeadline},
		{name: "cancellation wins failure", facts: TerminationFacts{Cancellation: cancellation, Outcome: failed}, want: StatusCancelled, cause: TerminationCauseParentCancellation},
		{name: "failure without control", facts: TerminationFacts{Outcome: failed}, want: StatusFailed, cause: TerminationCauseExternalFailure},
		{name: "completion", facts: TerminationFacts{Outcome: CompletedOutcome()}, want: StatusCompleted, cause: TerminationCauseCompletion},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ResolveTermination(test.facts)
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
	if _, err := ResolveTermination(TerminationFacts{}); !errors.Is(err, ErrInvalidTermination) {
		t.Fatalf("ResolveTermination(empty) error = %v, want ErrInvalidTermination", err)
	}
	if _, err := NewDeadlineIntent(DeadlineOwnerInvalid, "deadline"); !errors.Is(err, ErrInvalidTermination) {
		t.Fatalf("NewDeadlineIntent error = %v, want ErrInvalidTermination", err)
	}
}
