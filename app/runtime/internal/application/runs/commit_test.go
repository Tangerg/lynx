package runs

import (
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/itemfixture"
	runfixture "github.com/Tangerg/lynx/app/runtime/internal/testsupport/runfixture"
)

func TestEventCommitUsesCompleteRunStateInvariant(t *testing.T) {
	createdAt := time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC)
	waiting := runfixture.MustRestore(run.Snapshot{ID: "run_1", SessionID: "session", State: run.Waiting,
		CreatedAt: createdAt, UpdatedAt: createdAt,
		MessageMark: run.UnknownMessageMark})

	valid := EventCommit{
		RunID: waiting.ID(), SessionID: waiting.SessionID(), SegmentID: "segment_1",
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
		RunID: record.ID(), SessionID: record.SessionID(), SegmentID: "segment_1", State: StateTerminalize,
		CommitID: "event_commit_1", Outcome: outcome, Run: &record,
	}
	if err := commit.Validate(); err != nil {
		t.Fatalf("terminal commit awaiting transactional watermark: %v", err)
	}
	commit.CommitID = ""
	if err := commit.Validate(); err == nil {
		t.Fatal("terminal commit without an immutable commit identity passed validation")
	}

	invalid := record.Snapshot()
	invalid.MessageMark = run.UnknownMessageMark - 1
	if _, err := run.Restore(invalid); err == nil {
		t.Fatal("Run.Restore accepted an invalid negative message watermark")
	}
}

func TestEventCommitToolJournalOwnsMatchingItemState(t *testing.T) {
	startedAt := time.Date(2026, 8, 13, 2, 3, 4, 0, time.UTC)
	finishedAt := startedAt.Add(time.Second)
	running := itemfixture.MustRestore(itemfixture.Input{
		SessionID: "session", RunID: "run_1", ID: "item_1",
		Status: transcript.ItemRunning, Kind: transcript.ToolCall, OccurredAt: startedAt,
	})
	completed := itemfixture.MustRestore(itemfixture.Input{
		SessionID: "session", RunID: "run_1", ID: "item_1",
		Status: transcript.ItemCompleted, Kind: transcript.ToolCall,
		OccurredAt: startedAt, FinishedAt: finishedAt,
	})
	failed := itemfixture.MustRestore(itemfixture.Input{
		SessionID: "session", RunID: "run_1", ID: "item_1",
		Status: transcript.ItemIncomplete, Kind: transcript.ToolCall,
		OccurredAt: startedAt, FinishedAt: finishedAt,
		Failure: &tool.Failure{Kind: tool.FailureExecution},
	})
	unknown := itemfixture.MustRestore(itemfixture.Input{
		SessionID: "session", RunID: "run_1", ID: "item_1",
		Status: transcript.ItemIncomplete, Kind: transcript.ToolCall,
		OccurredAt: startedAt, FinishedAt: finishedAt,
	})
	started := ToolInvocationCommit{
		CallID: "call_1", ItemID: running.ID(), SegmentID: "segment_1",
		State: ToolInvocationStarted, StartedAt: startedAt,
	}
	terminal := started
	terminal.State = ToolInvocationCompleted
	terminal.FinishedAt = finishedAt

	tests := []struct {
		name       string
		items      []transcript.Item
		invocation ToolInvocationCommit
		wantErr    bool
	}{
		{name: "missing Item", invocation: started, wantErr: true},
		{name: "started with terminal Item", items: []transcript.Item{completed}, invocation: started, wantErr: true},
		{name: "completed with running Item", items: []transcript.Item{running}, invocation: terminal, wantErr: true},
		{name: "completed with unclassified Item", items: []transcript.Item{unknown}, invocation: terminal, wantErr: true},
		{name: "matched start", items: []transcript.Item{running}, invocation: started},
		{name: "matched completion", items: []transcript.Item{completed}, invocation: terminal},
		{name: "matched failed completion", items: []transcript.Item{failed}, invocation: terminal},
		{name: "different Segment", items: []transcript.Item{running}, invocation: ToolInvocationCommit{
			CallID: "call_1", ItemID: running.ID(), SegmentID: "segment_2",
			State: ToolInvocationStarted, StartedAt: startedAt,
		}, wantErr: true},
		{name: "parked attempt", items: []transcript.Item{running}, invocation: ToolInvocationCommit{
			CallID: "call_1", ItemID: running.ID(), SegmentID: "segment_1",
			State: ToolInvocationIncomplete, StartedAt: startedAt, FinishedAt: finishedAt,
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := (EventCommit{
				RunID: "run_1", SessionID: "session", SegmentID: "segment_1", Items: test.items,
				ToolInvocations: []ToolInvocationCommit{test.invocation},
			}).Validate()
			if (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestEventCommitOwnsInvocationAndProgressSegment(t *testing.T) {
	startedAt := time.Date(2026, 8, 15, 1, 2, 3, 0, time.UTC)
	commit := EventCommit{
		RunID: "run_1", SessionID: "session", SegmentID: "segment_1",
		ModelInvocations: []ModelInvocationCommit{{
			CallID: "call_1", SegmentID: "segment_2",
			State: ModelInvocationStarted, StartedAt: startedAt,
		}},
	}
	if err := commit.Validate(); err == nil {
		t.Fatal("EventCommit accepted a model invocation from another Segment")
	}
	commit.ModelInvocations = nil
	commit.Progress = &RunProgressCommit{
		SegmentID: "segment_2", Metrics: run.Metrics{}, UpdatedAt: startedAt,
	}
	if err := commit.Validate(); err == nil {
		t.Fatal("EventCommit accepted progress from another Segment")
	}
}
