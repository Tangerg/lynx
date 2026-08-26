// Package runfixture provides test-only construction helpers for valid Run
// aggregates. Production code must use the domain constructors directly.
package runfixture

import (
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
)

// MetricsInput names the values accepted by MustMetrics.
type MetricsInput struct {
	Usage          *accounting.Usage
	Steps          int
	ActiveDuration time.Duration
}

// Selection returns the deterministic model identity MustRestore supplies when
// a fixture omits one.
func Selection() modelref.Selection {
	selection, _ := modelref.New("fixture", "fixture")
	return selection
}

// MustMetrics constructs valid metrics or panics. It is intended only for
// fixtures whose validity is not the behavior under test.
func MustMetrics(input MetricsInput) run.Metrics {
	metrics, err := run.NewMetrics(input.Usage, input.Steps, input.ActiveDuration)
	if err != nil {
		panic(err)
	}
	return metrics
}

// MustRestore constructs a valid Run or panics. Tests exercising invalid
// snapshots must call run.Restore themselves and assert the returned error.
func MustRestore(snapshot run.Snapshot) run.Run {
	if snapshot.ID == "" {
		snapshot.ID = "run_fixture"
	}
	if snapshot.SessionID == "" {
		snapshot.SessionID = "session_fixture"
	}
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Unix(1, 0).UTC()
	}
	if snapshot.State == "" {
		snapshot.State = run.Running
	}
	if snapshot.Outcome != nil && snapshot.State == run.Running {
		if terminal, ok := run.Running.Terminate(*snapshot.Outcome); ok {
			snapshot.State = terminal
		}
	}
	if snapshot.State.IsTerminal() && snapshot.Outcome == nil {
		var outcome run.Outcome
		switch snapshot.State {
		case run.Completed:
			outcome = run.OutcomeCompleted
		case run.Canceled:
			outcome = run.OutcomeCanceled
		case run.Failed:
			outcome = run.OutcomeFailed
		}
		snapshot.Outcome = &outcome
	}
	if snapshot.State == run.Running && snapshot.ActiveSegmentID == "" {
		snapshot.ActiveSegmentID = "segment_fixture"
	}
	if snapshot.State != run.Running {
		snapshot.ActiveSegmentID = ""
	}
	if snapshot.State.IsTerminal() {
		if snapshot.FinishedAt.IsZero() {
			snapshot.FinishedAt = snapshot.CreatedAt
		}
		if snapshot.Failure == nil {
			switch *snapshot.Outcome {
			case run.OutcomeFailed:
				snapshot.Failure = &run.Failure{Kind: run.FailureInternal}
			case run.OutcomeTimedOut:
				snapshot.Failure = &run.Failure{Kind: run.FailureTimeout}
			case run.OutcomeLost:
				snapshot.Failure = &run.Failure{Kind: run.FailureLost}
			}
		}
	} else {
		snapshot.MessageMark = run.UnknownMessageMark
	}
	if snapshot.UpdatedAt.IsZero() {
		if !snapshot.FinishedAt.IsZero() {
			snapshot.UpdatedAt = snapshot.FinishedAt
		} else {
			snapshot.UpdatedAt = snapshot.CreatedAt
		}
	}
	restored, err := run.Restore(snapshot)
	if err != nil {
		panic(err)
	}
	return restored
}
