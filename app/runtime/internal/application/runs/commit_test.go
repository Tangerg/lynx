package runs

import (
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	runfixture "github.com/Tangerg/lynx/app/runtime/internal/testsupport/runfixture"
)

func TestEventCommitUsesCompleteRunStateInvariant(t *testing.T) {
	createdAt := time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC)
	waiting := runfixture.MustRestore(run.Snapshot{ID: "run_1", SessionID: "session", State: run.Waiting,
		CreatedAt: createdAt, UpdatedAt: createdAt,
		MessageMark: run.UnknownMessageMark})

	valid := EventCommit{
		RunID: waiting.ID(), SessionID: waiting.SessionID(),
		State: StateSuspend, Run: &waiting,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid suspend commit: %v", err)
	}

	contradictory := waiting.Snapshot()
	contradictory.ActiveSegmentID = "segment_stale"
	if _, err := run.Restore(contradictory); err == nil {
		t.Fatal("Run.Restore accepted a waiting Run with an active Segment")
	}
}

func TestTerminalEventCommitAllowsOnlyTheTransactionalWatermarkPlaceholder(t *testing.T) {
	createdAt := time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC)
	outcome := run.OutcomeCanceled
	record := runfixture.MustRestore(run.Snapshot{ID: "run_1", SessionID: "session", State: run.Canceled,
		Outcome: &outcome, CreatedAt: createdAt, UpdatedAt: createdAt.Add(time.Second),
		FinishedAt: createdAt.Add(time.Second), MessageMark: run.UnknownMessageMark})

	commit := EventCommit{
		RunID: record.ID(), SessionID: record.SessionID(), State: StateTerminalize,
		Outcome: outcome, Run: &record,
	}
	if err := commit.Validate(); err != nil {
		t.Fatalf("terminal commit awaiting transactional watermark: %v", err)
	}

	invalid := record.Snapshot()
	invalid.MessageMark = run.UnknownMessageMark - 1
	if _, err := run.Restore(invalid); err == nil {
		t.Fatal("Run.Restore accepted an invalid negative message watermark")
	}
}
