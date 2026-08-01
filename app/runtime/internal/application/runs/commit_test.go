package runs

import (
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

func TestEventCommitUsesCompleteRunStateInvariant(t *testing.T) {
	createdAt := time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC)
	interrupted := transcript.Run{
		ID: "run_1", SessionID: "session", State: execution.Interrupted,
		CreatedAt: createdAt, UpdatedAt: createdAt,
		MessageMark: transcript.UnknownMessageMark,
	}
	valid := EventCommit{
		RunID: interrupted.ID, SessionID: interrupted.SessionID,
		State: StateSuspend, Run: &interrupted,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid suspend commit: %v", err)
	}

	contradictory := interrupted
	contradictory.ActiveSegmentID = "segment_stale"
	invalid := valid
	invalid.Run = &contradictory
	if err := invalid.Validate(); err == nil {
		t.Fatal("EventCommit.Validate accepted an interrupted Run with an active Segment")
	}
}

func TestTerminalEventCommitAllowsOnlyTheTransactionalWatermarkPlaceholder(t *testing.T) {
	createdAt := time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC)
	outcome := execution.OutcomeCanceled
	run := transcript.Run{
		ID: "run_1", SessionID: "session", State: execution.Canceled,
		Outcome: &outcome, CreatedAt: createdAt, UpdatedAt: createdAt.Add(time.Second),
		FinishedAt: createdAt.Add(time.Second), MessageMark: transcript.UnknownMessageMark,
	}
	commit := EventCommit{
		RunID: run.ID, SessionID: run.SessionID, State: StateTerminalize,
		Outcome: outcome, Run: &run,
	}
	if err := commit.Validate(); err != nil {
		t.Fatalf("terminal commit awaiting transactional watermark: %v", err)
	}

	run.MessageMark = transcript.UnknownMessageMark - 1
	if err := commit.Validate(); err == nil {
		t.Fatal("EventCommit.Validate accepted an invalid negative message watermark")
	}
}
