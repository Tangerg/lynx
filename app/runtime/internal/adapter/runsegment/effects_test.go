package runsegment

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/toolresult"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/itemfixture"
	runfixture "github.com/Tangerg/lynx/app/runtime/internal/testsupport/runfixture"
)

var (
	runsegmentTraceOnce     sync.Once
	runsegmentTraceExporter *tracetest.InMemoryExporter
	runsegmentTraceProvider *sdktrace.TracerProvider
)

func installRunsegmentTraceCapture(t *testing.T) (*sdktrace.TracerProvider, *tracetest.InMemoryExporter) {
	t.Helper()
	runsegmentTraceOnce.Do(func() {
		runsegmentTraceExporter = tracetest.NewInMemoryExporter()
		runsegmentTraceProvider = sdktrace.NewTracerProvider(sdktrace.WithSyncer(runsegmentTraceExporter))
		otel.SetTracerProvider(runsegmentTraceProvider)
	})
	runsegmentTraceExporter.Reset()
	t.Cleanup(runsegmentTraceExporter.Reset)
	return runsegmentTraceProvider, runsegmentTraceExporter
}

func mustEffectSelection(t testing.TB, provider, model string) modelref.Selection {
	t.Helper()
	selection, err := modelref.New(provider, model)
	if err != nil {
		t.Fatalf("modelref.New(%q, %q): %v", provider, model, err)
	}
	return selection
}

func singleRunPending(
	t testing.TB,
	runID, sessionID, memberID, requestID, itemID string,
	runCreatedAt, barrierCreatedAt time.Time,
) runs.Pending {
	t.Helper()
	question := &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?"}}}
	return runs.Pending{
		RootRunID:  runID,
		SessionID:  sessionID,
		ExecutorID: "turn_" + runID,
		Capabilities: run.Capabilities{
			InterruptKinds: []interrupt.Kind{interrupt.Question},
		},
		Interrupts: []transcript.Interrupt{{
			ItemID: itemID, ItemOccurredAt: barrierCreatedAt,
			RunID:    runID,
			Kind:     interrupt.Question,
			Question: question,
		}},
		Bindings: []runs.InterruptBinding{{
			InterruptItemID: itemID,
			MemberID:        memberID,
			RequestID:       requestID,
		}},
		Continuations: []runs.Continuation{{
			RunID:          runID,
			MemberID:       memberID,
			ModelSelection: mustEffectSelection(t, "anthropic", "claude"),
			RunCreatedAt:   runCreatedAt,
		}},
		CreatedAt: barrierCreatedAt,
	}
}

// TestCommitEventPersistsTranscriptAndTerminalizes: a terminal commit appends the
// item projection AND terminalizes the Run with its resolved message watermark —
// all through one CommitEvent, atomically inside the wired transactor.
func TestCommitEventPersistsTranscriptAndTerminalizes(t *testing.T) {
	stores := &fakeStores{transcript: &fakeTranscript{}, mark: 7}
	runState := &fakeRunState{}
	tx := &fakeTx{}
	effects := testEffects(stores, Config{State: runState, Tx: tx.run})

	err := effects.CommitEvent(t.Context(), runs.EventCommit{
		RunID:     "run_1",
		SessionID: "ses_1",
		State:     runs.StateTerminalize,
		Outcome:   run.OutcomeCompleted,
		Items: []transcript.Item{itemfixture.MustRestore(itemfixture.Input{
			SessionID: "ses_1", RunID: "run_1", ID: "item_1", OccurredAt: time.Unix(1, 0).UTC(),
		})},
		Run: finishedRunRecord("run_1", "ses_1", run.OutcomeCompleted),
	})
	if err != nil {
		t.Fatalf("CommitEvent: %v", err)
	}

	if len(stores.transcript.items) != 1 || stores.transcript.items[0].ID() != "item_1" {
		t.Fatalf("items = %+v, want item_1", stores.transcript.items)
	}
	if len(runState.terminalized) != 1 {
		t.Fatalf("terminalized = %+v, want one finished run", runState.terminalized)
	}
	finished := runState.terminalized[0]
	if finished.SessionID() != "ses_1" || finished.ID() != "run_1" || !runHasOutcome(finished, run.OutcomeCompleted) {
		t.Fatalf("terminalized run = %+v", finished)
	}
	if finished.MessageMark() != 7 {
		t.Fatalf("terminalized mark = %d, want the resolved 7", finished.MessageMark())
	}
	if tx.calls != 1 {
		t.Fatalf("RunInTx calls = %d, want 1 (the whole commit is one transaction)", tx.calls)
	}
}

func TestCommitEventBindsOffloadedResultWithTranscriptItem(t *testing.T) {
	toolResults := new(fakeToolResults)
	stores := &fakeStores{transcript: new(fakeTranscript), toolResults: toolResults}
	effects := testEffects(stores, Config{Tx: new(fakeTx).run})
	ref := &toolresult.Ref{ID: "BLOB234"}
	preview := tool.StringResult("preview")

	err := effects.CommitEvent(t.Context(), runs.EventCommit{
		RunID: "run_1", SessionID: "ses_1",
		Items: []transcript.Item{itemfixture.MustRestore(itemfixture.Input{
			SessionID: "ses_1", RunID: "run_1", ID: "item_1",
			Kind: transcript.ToolCall, Status: transcript.ItemCompleted,
			OccurredAt: time.Unix(1, 0).UTC(), FinishedAt: time.Unix(2, 0).UTC(),
			Tool: &transcript.ToolInvocation{Name: "shell", Result: &preview, Offload: ref},
		})},
	})
	if err != nil {
		t.Fatalf("CommitEvent: %v", err)
	}
	if len(toolResults.bindings) != 1 {
		t.Fatalf("bindings = %+v, want one", toolResults.bindings)
	}
	got := toolResults.bindings[0]
	if got.sessionID != "ses_1" || got.itemID != "item_1" || got.preview != "preview" || got.ref != *ref {
		t.Fatalf("binding = %+v, want exact item/ref", got)
	}
}

func TestCommitEventDiscardsStagedOffloadAfterCommitFailure(t *testing.T) {
	want := errors.New("transaction failed")
	toolResults := new(fakeToolResults)
	stores := &fakeStores{transcript: new(fakeTranscript), toolResults: toolResults}
	effects := testEffects(stores, Config{
		Tx: func(context.Context, func(context.Context) error) error {
			return want
		},
	})
	ref := &toolresult.Ref{ID: "BLOB234"}
	preview := tool.StringResult("preview")

	err := effects.CommitEvent(t.Context(), runs.EventCommit{
		RunID: "run_1", SessionID: "ses_1",
		Items: []transcript.Item{itemfixture.MustRestore(itemfixture.Input{
			SessionID: "ses_1", RunID: "run_1", ID: "item_1",
			Kind: transcript.ToolCall, Status: transcript.ItemCompleted,
			OccurredAt: time.Unix(1, 0).UTC(), FinishedAt: time.Unix(2, 0).UTC(),
			Tool: &transcript.ToolInvocation{Name: "shell", Result: &preview, Offload: ref},
		})},
	})
	if !errors.Is(err, want) {
		t.Fatalf("CommitEvent error = %v, want %v", err, want)
	}
	if len(toolResults.discarded) != 1 || toolResults.discarded[0].sessionID != "ses_1" || toolResults.discarded[0].ref != *ref {
		t.Fatalf("discarded = %+v, want exact staged blob", toolResults.discarded)
	}
}

func TestCommitEventRejectsUnresolvedTerminalMessageWatermark(t *testing.T) {
	want := errors.New("message count unavailable")
	stores := &fakeStores{transcript: &fakeTranscript{}, markErr: want}
	runState := &fakeRunState{}
	effects := testEffects(stores, Config{State: runState, Tx: new(fakeTx).run})

	err := effects.CommitEvent(t.Context(), runs.EventCommit{
		RunID:     "run_1",
		SessionID: "ses_1",
		State:     runs.StateTerminalize,
		Outcome:   run.OutcomeCompleted,
		Run:       finishedRunRecord("run_1", "ses_1", run.OutcomeCompleted),
	})
	if !errors.Is(err, want) {
		t.Fatalf("CommitEvent error = %v, want %v", err, want)
	}
	if len(runState.terminalized) != 0 {
		t.Fatalf("terminalized = %+v, want none", runState.terminalized)
	}
}

func TestCommitEventRejectsUnknownStateChange(t *testing.T) {
	effects := testEffects(&fakeStores{transcript: &fakeTranscript{}}, Config{
		State: &fakeRunState{},
		Tx:    new(fakeTx).run,
	})
	err := effects.CommitEvent(t.Context(), runs.EventCommit{
		RunID: "run_1", SessionID: "ses_1", State: runs.StateChange(255),
		Run: runPointer(runfixture.MustRestore(run.Snapshot{SessionID: "ses_1", ID: "run_1"})),
	})
	if err == nil {
		t.Fatal("CommitEvent accepted an unknown run state change")
	}
}

func TestCommitOpeningAdmitsAndProjectsInOneTransaction(t *testing.T) {
	stores := &fakeStores{transcript: &fakeTranscript{}}
	runState := &fakeRunState{}
	tx := &fakeTx{}
	effects := testEffects(stores, Config{State: runState, Tx: tx.run})
	draft := run.Draft{RunID: "run_1", SessionID: "ses_1", SegmentID: "seg_open"}

	err := effects.CommitOpening(context.Background(), runs.OpeningCommit{
		Admit: &draft,
		Events: []runs.EventCommit{{
			RunID:     "run_1",
			SessionID: "ses_1",
			Items: []transcript.Item{itemfixture.MustRestore(itemfixture.Input{
				SessionID: "ses_1", RunID: "run_1", ID: "item_1", OccurredAt: time.Unix(1, 0).UTC(),
			})},
		}},
	})
	if err != nil {
		t.Fatalf("CommitOpening: %v", err)
	}
	if tx.calls != 1 || len(runState.admitted) != 1 || len(stores.transcript.items) != 1 {
		t.Fatalf("opening tx=%d admitted=%d items=%d, want 1/1/1", tx.calls, len(runState.admitted), len(stores.transcript.items))
	}
}

func TestCommitStartedChildRunOwnsOneTransactionBoundary(t *testing.T) {
	startedAt := time.Unix(2, 0).UTC()
	reservation := runs.ChildRunStartReservation{
		SessionID:  "ses_1",
		ExecutorID: "executor_1",
		Member: runs.ExecutorMember{
			MemberID: "member_child", ParentID: "member_root", SpawnCallID: "call_1",
		},
		Binding: runs.ChildRunBinding{
			MemberID: "member_child", RunID: "run_child", ParentRunID: "run_root",
		},
		SegmentID:       "segment_child",
		SpawnedByItemID: "item_delegate",
		RootRunID:       "run_root",
		StartedAt:       startedAt,
	}
	draft := run.Draft{
		RunID: "run_child", SessionID: "ses_1", SegmentID: "segment_child",
		SpawnedByItemID: "item_delegate", ParentRunID: "run_root", RootRunID: "run_root",
		CreatedAt: startedAt,
	}
	tx := &nonReentrantTx{}
	childStarts := &fakeChildRunStarts{}
	runState := &fakeRunState{}
	effects := testEffects(&fakeStores{transcript: &fakeTranscript{}}, Config{
		State: runState, ChildRunStarts: childStarts, Tx: tx.run,
	})

	if err := effects.CommitStartedChildRun(t.Context(), reservation, runs.OpeningCommit{
		Admit: &draft,
	}); err != nil {
		t.Fatalf("CommitStartedChildRun: %v", err)
	}
	if tx.calls != 1 {
		t.Fatalf("transaction entries = %d, want one composite boundary", tx.calls)
	}
	if childStarts.conclusions != 1 || len(runState.admitted) != 1 {
		t.Fatalf(
			"conclusions=%d admitted=%d, want one reservation conclusion and one admission",
			childStarts.conclusions, len(runState.admitted),
		)
	}
}

func TestChildRunStartReservationUsesAdapterOwnedCanonicalPayload(t *testing.T) {
	startedAt := time.Date(2026, time.August, 10, 3, 4, 5, 6, time.FixedZone("offset", 8*60*60))
	record, err := childRunStartReservationRecord(runs.ChildRunStartReservation{
		SessionID:  "ses_1",
		ExecutorID: "executor_1",
		Member: runs.ExecutorMember{
			MemberID: "member_child", ParentID: "member_root", SpawnCallID: "call_1",
		},
		Binding: runs.ChildRunBinding{
			MemberID: "member_child", RunID: "run_child", ParentRunID: "run_root",
		},
		SegmentID:       "segment_child",
		SpawnedByItemID: "item_delegate",
		RootRunID:       "run_root",
		StartedAt:       startedAt,
	})
	if err != nil {
		t.Fatalf("childRunStartReservationRecord: %v", err)
	}
	if record.MemberID != "member_child" || record.SessionID != "ses_1" ||
		!record.CreatedAt.Equal(startedAt.UTC()) {
		t.Fatalf("technical record identity = %+v", record)
	}
	wantPayload := `{"executorId":"executor_1","parentMemberId":"member_root","spawnCallId":"call_1","runId":"run_child","parentRunId":"run_root","segmentId":"segment_child","spawnedByItemId":"item_delegate","rootRunId":"run_root"}`
	if got := string(record.Payload); got != wantPayload {
		t.Fatalf("reservation payload = %s, want %s", got, wantPayload)
	}
}

func TestCommitOpeningResumesAfterSeparateAnswerClaim(t *testing.T) {
	now := time.Now().UTC()
	ints := &fakeInterrupts{pending: singleRunPending(
		t, "run_1", "ses_1", "member_1", "request_1", "item_1", now, now,
	), resumeClaimed: true}
	stores := &fakeStores{interrupts: ints, transcript: &fakeTranscript{}}
	runState := &fakeRunState{}
	tx := &fakeTx{}
	effects := testEffects(stores, Config{State: runState, Tx: tx.run})
	resume := run.TreeResumeDraft{
		RootRunID: "run_1",
		SessionID: "ses_1",
		ResumedAt: now,
		Runs:      []run.ResumeDraft{{RunID: "run_1", SegmentID: "seg_next"}},
	}

	err := effects.CommitOpening(context.Background(), runs.OpeningCommit{
		Resume: &resume,
		Events: []runs.EventCommit{{
			RunID:     "run_1",
			SessionID: "ses_1",
			Items: []transcript.Item{itemfixture.MustRestore(itemfixture.Input{
				SessionID: "ses_1", RunID: "run_1", ID: "item_1", OccurredAt: now,
			})},
		}},
	})
	if err != nil {
		t.Fatalf("CommitOpening: %v", err)
	}
	if tx.calls != 1 || ints.pending.RootRunID == "" || len(runState.resumed) != 1 || len(stores.transcript.items) != 1 {
		t.Fatalf("resume tx=%d pending=%+v resumed=%v items=%d", tx.calls, ints.pending, runState.resumed, len(stores.transcript.items))
	}
}

// TestCommitTreeBarrierRecordsPendingSetAndSuspends: one tree barrier persists
// the complete executor hand-off beside the waiting Run — atomically.
func TestCommitTreeBarrierRecordsPendingSetAndSuspends(t *testing.T) {
	stores := &fakeStores{interrupts: &fakeInterrupts{}, transcript: &fakeTranscript{}}
	runState := &fakeRunState{}
	tx := &fakeTx{}
	effects := testEffects(stores, Config{State: runState, Tx: tx.run})
	runCreatedAt := time.Unix(1, 0).UTC()
	barrierCreatedAt := time.Unix(2, 0).UTC()
	pending := singleRunPending(
		t,
		"run_1", "ses_1", "member_1", "request_1", "int_1",
		runCreatedAt, barrierCreatedAt,
	)
	pending.Continuations[0].DrainedTools = []runs.DrainedTool{{
		ItemID: "tool_1", ItemOccurredAt: barrierCreatedAt,
		CallID: "call_1", Name: "ask_user", Arguments: "{}",
	}}
	pending.Continuations[0].Metrics = runfixture.MustMetrics(runfixture.MetricsInput{Steps: 2})

	err := effects.CommitTreeBarrier(context.Background(), runs.TreeBarrierCommit{
		Pending:    pending,
		Checkpoint: testRootExecutorCheckpoint(),
		Runs: []runs.EventCommit{{
			RunID:     "run_1",
			SessionID: "ses_1",
			State:     runs.StateSuspend,
			Run: runPointer(runfixture.MustRestore(run.Snapshot{SessionID: "ses_1", ID: "run_1", State: run.Waiting,
				ModelSelection: pending.Continuations[0].ModelSelection,

				Metrics:      runfixture.MustMetrics(runfixture.MetricsInput{Steps: 2}),
				Capabilities: pending.Capabilities,
				CreatedAt:    runCreatedAt,
				UpdatedAt:    barrierCreatedAt,
				MessageMark:  run.UnknownMessageMark})),

			Items: []transcript.Item{itemfixture.MustRestore(itemfixture.Input{
				SessionID: "ses_1", RunID: "run_1", ID: "int_1",
				Kind:       transcript.QuestionItem,
				OccurredAt: barrierCreatedAt, Question: pending.Interrupts[0].Question,
			})},
		}},
	})
	if err != nil {
		t.Fatalf("CommitTreeBarrier: %v", err)
	}

	got := stores.interrupts.pending
	root, ok := got.RootContinuation()
	if got.RootRunID != "run_1" || !ok || root.MemberID != "member_1" ||
		root.ModelSelection.Provider() != "anthropic" || root.ModelSelection.Model() != "claude" {
		t.Fatalf("pending = %+v", got)
	}
	if len(got.Interrupts) != 1 || got.Interrupts[0].ItemID != "int_1" || len(root.DrainedTools) != 1 {
		t.Fatalf("pending interrupts = %+v drained=%+v", got.Interrupts, root.DrainedTools)
	}
	if len(runState.suspended) != 1 || runState.suspended[0].ID() != "run_1" || runState.suspended[0].Metrics().Steps() != 2 {
		t.Fatalf("suspended = %+v, want run_1 with the accrual the park froze", runState.suspended)
	}
	if len(stores.transcript.items) != 1 || stores.transcript.items[0].ID() != "int_1" {
		t.Fatalf("park transcript = items:%+v, want one running interrupt item", stores.transcript.items)
	}
}

// TestCommitTreeBarrierRejectsIncompleteContinuation: the barrier owns exact
// executor identities. An incomplete continuation fails before the transaction,
// so no partial pending set or Run transition is visible.
func TestCommitTreeBarrierRejectsIncompleteContinuation(t *testing.T) {
	stores := &fakeStores{interrupts: &fakeInterrupts{}}
	runState := &fakeRunState{}
	tx := &fakeTx{}
	effects := testEffects(stores, Config{State: runState, Tx: tx.run})
	createdAt := time.Unix(1, 0).UTC()
	pending := singleRunPending(t, "run_1", "ses_1", "member_1", "request_1", "int_1", createdAt, createdAt.Add(time.Second))
	pending.Continuations[0].MemberID = ""

	err := effects.CommitTreeBarrier(context.Background(), runs.TreeBarrierCommit{
		Pending:    pending,
		Checkpoint: testRootExecutorCheckpoint(),
		Runs: []runs.EventCommit{{
			RunID: "run_1", SessionID: "ses_1", State: runs.StateSuspend,
			Run: runPointer(runfixture.MustRestore(run.Snapshot{SessionID: "ses_1", ID: "run_1", State: run.Waiting,
				CreatedAt:   createdAt,
				MessageMark: run.UnknownMessageMark})),
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "member id is required") {
		t.Fatalf("CommitTreeBarrier error = %v, want missing member id", err)
	}
	if stores.interrupts.pending.RootRunID != "" {
		t.Fatalf("invalid pending set was persisted: %+v", stores.interrupts.pending)
	}
	if len(runState.suspended) != 0 {
		t.Fatalf("suspended = %v, want none", runState.suspended)
	}
	if tx.calls != 0 {
		t.Fatalf("transactions = %d, want validation before transaction", tx.calls)
	}
}

func TestCommitTreeBarrierRejectsMismatchedCheckpointBindingBeforeTransaction(t *testing.T) {
	createdAt := time.Unix(1, 0).UTC()
	pending := singleRunPending(
		t,
		"run_1", "ses_1", "member_1", "request_1", "int_1",
		createdAt, createdAt.Add(time.Second),
	)
	for name, mutate := range map[string]func(*runs.ExecutorCheckpoint){
		"root":       func(checkpoint *runs.ExecutorCheckpoint) { checkpoint.RootMemberID = "other_proc" },
		"session":    func(checkpoint *runs.ExecutorCheckpoint) { checkpoint.Scope.SessionID = "other_session" },
		"goal lease": func(checkpoint *runs.ExecutorCheckpoint) { checkpoint.Scope.GoalLeaseID = "other_goal" },
		"limits":     func(checkpoint *runs.ExecutorCheckpoint) { checkpoint.Limits.MaxTotalTokens++ },
		"provider": func(checkpoint *runs.ExecutorCheckpoint) {
			checkpoint.ModelSelection, _ = modelref.New("openai", checkpoint.ModelSelection.Model())
		},
		"model": func(checkpoint *runs.ExecutorCheckpoint) {
			checkpoint.ModelSelection, _ = modelref.New(checkpoint.ModelSelection.Provider(), "other-model")
		},
	} {
		t.Run(name, func(t *testing.T) {
			stores := &fakeStores{interrupts: &fakeInterrupts{}}
			tx := &fakeTx{}
			effects := testEffects(stores, Config{State: &fakeRunState{}, Tx: tx.run})
			checkpoint := testRootExecutorCheckpoint()
			mutate(&checkpoint)
			err := effects.CommitTreeBarrier(t.Context(), runs.TreeBarrierCommit{
				Pending:    pending,
				Checkpoint: checkpoint,
				Runs: []runs.EventCommit{{
					RunID: "run_1", SessionID: "ses_1", State: runs.StateSuspend,
					Run: runPointer(runfixture.MustRestore(run.Snapshot{SessionID: "ses_1", ID: "run_1", State: run.Waiting,
						CreatedAt:   createdAt,
						MessageMark: run.UnknownMessageMark})),
				}},
			})
			if !errors.Is(err, runs.ErrInvalidExecutorCheckpoint) {
				t.Fatalf("CommitTreeBarrier error = %v, want ErrInvalidExecutorCheckpoint", err)
			}
			if tx.calls != 0 {
				t.Fatalf("transactions = %d, want validation before transaction", tx.calls)
			}
		})
	}
}

// TestCommitTreeBarrierRejectsRunContinuationFactDriftBeforeTransaction proves
// parked_continuation_matches_run_facts at the segment-event boundary: the Run
// projection and hand-off must be one fact before the transaction can start.
func TestCommitTreeBarrierRejectsRunContinuationFactDriftBeforeTransaction(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*runs.Pending, *run.Run)
	}{
		{
			name: "cumulative metrics",
			mutate: func(_ *runs.Pending, record *run.Run) {
				snapshot := record.Snapshot()
				snapshot.Metrics = runfixture.MustMetrics(runfixture.MetricsInput{Steps: snapshot.Metrics.Steps() + 1})
				*record = runfixture.MustRestore(snapshot)
			},
		},
		{
			name: "frozen limits",
			mutate: func(_ *runs.Pending, record *run.Run) {
				snapshot := record.Snapshot()
				snapshot.Limits.MaxSteps++
				*record = runfixture.MustRestore(snapshot)
			},
		},
		{
			name: "frozen model selection",
			mutate: func(_ *runs.Pending, record *run.Run) {
				snapshot := record.Snapshot()
				snapshot.ModelSelection = mustEffectSelection(t, "openai", "gpt")
				*record = runfixture.MustRestore(snapshot)
			},
		},
		{
			name: "frozen run capabilities",
			mutate: func(_ *runs.Pending, record *run.Run) {
				snapshot := record.Snapshot()
				snapshot.Capabilities.ChildRuns = true
				*record = runfixture.MustRestore(snapshot)
			},
		},
		{
			name: "root goal lease",
			mutate: func(_ *runs.Pending, record *run.Run) {
				snapshot := record.Snapshot()
				snapshot.GoalLeaseID = "other-lease"
				*record = runfixture.MustRestore(snapshot)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			createdAt := time.Unix(1, 0).UTC()
			pending := singleRunPending(
				t,
				"run_1", "ses_1", "member_1", "request_1", "int_1",
				createdAt, createdAt.Add(time.Second),
			)
			pending.GoalLeaseID = "goal-lease"
			pending.Continuations[0].Metrics = runfixture.MustMetrics(runfixture.MetricsInput{Steps: 2})
			pending.Continuations[0].Limits = run.Limits{MaxSteps: 5}
			run := runfixture.MustRestore(run.Snapshot{SessionID: pending.SessionID,
				ID:             pending.RootRunID,
				ModelSelection: pending.Continuations[0].ModelSelection,
				GoalLeaseID:    pending.GoalLeaseID,
				State:          run.Waiting,

				Metrics:      pending.Continuations[0].Metrics,
				Limits:       pending.Continuations[0].Limits,
				Capabilities: pending.Capabilities,
				CreatedAt:    createdAt,
				MessageMark:  run.UnknownMessageMark})

			test.mutate(&pending, &run)
			checkpoint := testRootExecutorCheckpoint()
			checkpoint.Scope.GoalLeaseID = pending.GoalLeaseID
			checkpoint.Limits = pending.Continuations[0].Limits
			stores := &fakeStores{interrupts: &fakeInterrupts{}}
			tx := &fakeTx{}
			effects := testEffects(stores, Config{State: &fakeRunState{}, Tx: tx.run})

			err := effects.CommitTreeBarrier(t.Context(), runs.TreeBarrierCommit{
				Pending: pending, Checkpoint: checkpoint,
				Runs: []runs.EventCommit{{
					RunID: run.ID(), SessionID: run.SessionID(),
					State: runs.StateSuspend, Run: &run,
				}},
			})
			if err == nil {
				t.Fatal("CommitTreeBarrier accepted contradictory Run and continuation facts")
			}
			if tx.calls != 0 || stores.interrupts.pending.RootRunID != "" {
				t.Fatalf("invalid barrier reached persistence: tx=%d pending=%+v", tx.calls, stores.interrupts.pending)
			}
		})
	}
}

// TestNudgePublishesFileChange: the non-durable workspace nudge reaches the
// publisher.
func TestNudgePublishesFileChange(t *testing.T) {
	var published struct {
		cwd   string
		paths []string
	}
	effects := New(Config{PublishFileChanges: func(change workspaceapp.FileChangeNotice) {
		published.cwd, published.paths = change.CWD, change.Paths
	}})

	effects.Nudge("/work", []string{"a.go"})
	if published.cwd != "/work" || len(published.paths) != 1 || published.paths[0] != "a.go" {
		t.Fatalf("published = %+v, want /work [a.go]", published)
	}
}

func TestFinishRunsTerminalMaintenanceOnlyForTerminalRuns(t *testing.T) {
	renamed := make(chan string, 1)
	snapshotted := make(chan string, 1)
	stores := &fakeStores{
		session: &fakeSession{
			sess:    session.Session{ID: "ses_1", CWD: "/repo"},
			renamed: renamed,
		},
		title: "Generated title",
	}
	effects := testEffects(stores, Config{
		Checkpoints: fakeCheckpoints{snapshotted: snapshotted},
	})

	effects.Finish(t.Context(), runs.Finish{SessionID: "ses_1", RunID: "run_1", CWD: "/run-cwd", OpeningUserText: "hello"})

	if got := waitString(t, snapshotted); got != "ses_1:/run-cwd:run_1" {
		t.Fatalf("snapshot = %q", got)
	}
	if got := waitString(t, renamed); got != "Generated title" {
		t.Fatalf("title = %q", got)
	}

	effects.Finish(t.Context(), runs.Finish{SessionID: "ses_1", RunID: "run_2", CWD: "/run-cwd", Parked: true, OpeningUserText: "ignored"})
	select {
	case got := <-snapshotted:
		t.Fatalf("parked run must not snapshot, got %q", got)
	case got := <-renamed:
		t.Fatalf("parked run must not title, got %q", got)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestFinishOrdersMaintenanceAndReportsEveryFailure(t *testing.T) {
	snapshotErr := errors.New("snapshot failed")
	renameErr := errors.New("rename failed")
	var operations []string
	stores := &fakeStores{
		session: &fakeSession{
			sess:       session.Session{ID: "ses_1"},
			operations: &operations,
			renameErr:  renameErr,
		},
		title:      "Generated title",
		operations: &operations,
	}
	effects := testEffects(stores, Config{
		Checkpoints: fakeCheckpoints{
			operations: &operations,
			err:        snapshotErr,
		},
	})

	err := effects.Finish(t.Context(), runs.Finish{
		SessionID:       "ses_1",
		RunID:           "run_1",
		CWD:             "/repo",
		OpeningUserText: "hello",
	})
	if !errors.Is(err, snapshotErr) || !errors.Is(err, renameErr) {
		t.Fatalf("Finish error = %v, want snapshot and rename failures", err)
	}
	want := []string{"checkpoint.snapshot", "session.get", "title.generate", "session.rename"}
	if !slices.Equal(operations, want) {
		t.Fatalf("operations = %v, want %v", operations, want)
	}
}

func TestFinishWaitsForCheckpointBeforeReturning(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	effects := New(Config{
		Checkpoints: fakeCheckpoints{started: started, release: release},
		Tasks:       inlineTaskLauncher{},
	})
	done := make(chan error, 1)
	go func() {
		done <- effects.Finish(t.Context(), runs.Finish{SessionID: "ses_1", RunID: "run_1", CWD: "/repo"})
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("checkpoint did not start")
	}
	select {
	case err := <-done:
		t.Fatalf("Finish returned before checkpoint completed: %v", err)
	default:
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("Finish: %v", err)
	}
}

func TestFinishRecordsAcceptedBackgroundFailureOnSpan(t *testing.T) {
	titleErr := errors.New("background title failed")
	provider, exporter := installRunsegmentTraceCapture(t)
	ctx, span := provider.Tracer("test/runsegment").Start(t.Context(), "run")
	effects := testEffects(&fakeStores{
		session:  &fakeSession{sess: session.Session{ID: "ses_1"}},
		titleErr: titleErr,
	}, Config{
		Tasks: inlineTaskLauncher{},
	})

	if err := effects.Finish(ctx, runs.Finish{SessionID: "ses_1", RunID: "run_1", OpeningUserText: "hello"}); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	span.End()

	for _, recorded := range exporter.GetSpans() {
		for _, event := range recorded.Events {
			for _, attr := range event.Attributes {
				if recorded.Name == "run terminal maintenance" && event.Name == "exception" && string(attr.Key) == "exception.message" && strings.Contains(attr.Value.AsString(), titleErr.Error()) {
					return
				}
			}
		}
	}
	t.Fatal("background maintenance failure was not recorded on the run span")
}

func waitString(t *testing.T, ch <-chan string) string {
	t.Helper()
	select {
	case got := <-ch:
		return got
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for async side effect")
		return ""
	}
}

type fakeStores struct {
	transcript  *fakeTranscript
	interrupts  *fakeInterrupts
	session     *fakeSession
	mark        int
	markErr     error
	title       string
	titleErr    error
	toolResults *fakeToolResults
	operations  *[]string
}

func testEffects(stores *fakeStores, cfg Config) *Effects {
	cfg.Interrupts = stores.interrupts
	cfg.ResumeClaims = stores.interrupts
	cfg.Sessions = stores.session
	cfg.Transcript = stores.transcript
	cfg.ToolResults = stores.toolResults
	cfg.Messages = stores
	cfg.Titles = stores
	if cfg.ExecutorCheckpoints == nil {
		cfg.ExecutorCheckpoints = &recordingExecutorCheckpointStore{}
	}
	return New(cfg)
}

func (s *fakeStores) Interrupts() InterruptStore   { return s.interrupts }
func (s *fakeStores) Session() SessionStore        { return s.session }
func (s *fakeStores) Transcript() TranscriptStore  { return s.transcript }
func (s *fakeStores) ToolResults() ToolResultStore { return s.toolResults }
func (s *fakeStores) Count(context.Context, string) (int, error) {
	return s.mark, s.markErr
}
func (s *fakeStores) Generate(context.Context, string) (string, error) {
	if s.operations != nil {
		*s.operations = append(*s.operations, "title.generate")
	}
	return s.title, s.titleErr
}

// finishedRunRecord is the terminal Run a reducer hands to a terminal commit: it
// carries its outcome and result but leaves the message watermark unknown, since
// resolving that is the committer's job.
func finishedRunRecord(runID, sessionID string, outcome run.Outcome) *run.Run {
	state, _ := run.Running.Terminate(outcome)
	record := runfixture.MustRestore(run.Snapshot{SessionID: sessionID, ID: runID, State: state, Outcome: &outcome,
		CreatedAt:   time.Unix(1, 0).UTC(),
		FinishedAt:  time.Unix(2, 0).UTC(),
		MessageMark: run.UnknownMessageMark})
	return &record
}

func runHasOutcome(record run.Run, expected run.Outcome) bool {
	outcome, terminal := record.Outcome()
	return terminal && outcome == expected
}

func mutatedRun(record run.Run, mutate func(*run.Snapshot)) run.Run {
	snapshot := record.Snapshot()
	mutate(&snapshot)
	return runfixture.MustRestore(snapshot)
}

func runPointer(record run.Run) *run.Run { return &record }

func mustResolveMessageMark(t *testing.T, record run.Run, mark int) run.Run {
	t.Helper()
	resolved, err := record.WithMessageMark(mark)
	if err != nil {
		t.Fatalf("resolve Run message mark: %v", err)
	}
	return resolved
}

// fakeRunState records the run-state transitions the commit applies.
type fakeRunState struct {
	admitted     []run.Draft
	resumed      []string
	suspended    []run.Run
	terminalized []run.Run
}

func (r *fakeRunState) Admit(_ context.Context, draft run.Draft) error {
	r.admitted = append(r.admitted, draft)
	return nil
}

func (r *fakeRunState) Resume(_ context.Context, sessionID string, _ run.ResumeDraft, _ time.Time) error {
	r.resumed = append(r.resumed, sessionID)
	return nil
}

func (r *fakeRunState) Suspend(_ context.Context, run run.Run) error {
	r.suspended = append(r.suspended, run)
	return nil
}

func (r *fakeRunState) Terminalize(_ context.Context, run run.Run) error {
	r.terminalized = append(r.terminalized, run)
	return nil
}

// fakeTx records how many transactions the commit opens and runs the body inline.
type fakeTx struct{ calls int }

func (t *fakeTx) run(ctx context.Context, fn func(context.Context) error) error {
	t.calls++
	return fn(ctx)
}

type nonReentrantTx struct {
	calls  int
	active bool
}

func (t *nonReentrantTx) run(ctx context.Context, fn func(context.Context) error) error {
	t.calls++
	if t.active {
		return errors.New("nested transaction")
	}
	t.active = true
	defer func() { t.active = false }()
	return fn(ctx)
}

type fakeChildRunStarts struct{ conclusions int }

func (*fakeChildRunStarts) Reserve(context.Context, sqlite.ChildRunStartReservationRecord) error {
	return nil
}

func (store *fakeChildRunStarts) Conclude(
	context.Context,
	sqlite.ChildRunStartReservationRecord,
	sqlite.ChildRunStartConclusion,
) (bool, error) {
	store.conclusions++
	return true, nil
}

type fakeTranscript struct {
	items []transcript.Item
}

type toolResultBinding struct {
	sessionID string
	itemID    string
	preview   string
	ref       toolresult.Ref
}

type fakeToolResults struct {
	bindings  []toolResultBinding
	discarded []toolResultBinding
}

func (s *fakeToolResults) Bind(_ context.Context, sessionID, itemID, preview string, ref toolresult.Ref) error {
	s.bindings = append(s.bindings, toolResultBinding{
		sessionID: sessionID, itemID: itemID, preview: preview, ref: ref,
	})
	return nil
}

func (s *fakeToolResults) Discard(_ context.Context, sessionID string, ref toolresult.Ref) error {
	s.discarded = append(s.discarded, toolResultBinding{sessionID: sessionID, ref: ref})
	return nil
}

func (s *fakeTranscript) AppendItem(_ context.Context, it transcript.Item) error {
	s.items = append(s.items, it)
	return nil
}

type fakeInterrupts struct {
	pending       runs.Pending
	resumeClaimed bool
}

func (s *fakeInterrupts) Open(_ context.Context, p runs.Pending) error {
	if s.pending.RootRunID != "" {
		return transcript.ErrIdentityConflict
	}
	s.pending = p
	return nil
}

func (s *fakeInterrupts) Consume(_ context.Context, sessionID, runID string) (runs.Pending, bool, error) {
	if s.pending.SessionID != sessionID || s.pending.RootRunID != runID {
		return runs.Pending{}, false, nil
	}
	pending := s.pending
	s.pending = runs.Pending{}
	return pending, true, nil
}

func (s *fakeInterrupts) Delete(_ context.Context, sessionID, runID string) error {
	if s.pending.RootRunID == "" {
		return nil
	}
	if s.pending.SessionID != sessionID || s.pending.RootRunID != runID {
		return transcript.ErrIdentityConflict
	}
	s.pending = runs.Pending{}
	return nil
}

func (s *fakeInterrupts) ClaimResume(
	_ context.Context,
	sessionID, runID string,
	_ []runs.InterruptAnswer,
	_ time.Time,
) (runs.Pending, bool, error) {
	if s.pending.SessionID != sessionID || s.pending.RootRunID != runID || s.resumeClaimed {
		return runs.Pending{}, false, nil
	}
	s.resumeClaimed = true
	return s.pending, true, nil
}

func (s *fakeInterrupts) RequireResumeClaim(_ context.Context, sessionID, runID string) error {
	if !s.resumeClaimed || s.pending.SessionID != sessionID || s.pending.RootRunID != runID {
		return errors.New("fake: resume claim is unavailable")
	}
	return nil
}

type fakeSession struct {
	sess       session.Session
	renamed    chan string
	operations *[]string
	getErr     error
	modelErr   error
	renameErr  error
}

func (s *fakeSession) List(context.Context) ([]session.Session, error) { return nil, nil }

func (s *fakeSession) Get(_ context.Context, id string) (session.Session, error) {
	if s.operations != nil {
		*s.operations = append(*s.operations, "session.get")
	}
	if s.getErr != nil {
		return session.Session{}, s.getErr
	}
	if id != s.sess.ID {
		return session.Session{}, session.ErrNotFound
	}
	return s.sess, nil
}

func (s *fakeSession) Ensure(_ context.Context, sess session.Session) (session.Session, error) {
	if s.sess.ID == "" {
		s.sess = sess
	}
	if s.sess.ID != sess.ID {
		return session.Session{}, session.ErrNotFound
	}
	return s.sess, nil
}

func (s *fakeSession) SetModel(_ context.Context, id, model string) error {
	if id != s.sess.ID {
		return session.ErrNotFound
	}
	if s.modelErr != nil {
		return s.modelErr
	}
	s.sess.Model = model
	return nil
}

func (s *fakeSession) RenameIfUntitled(_ context.Context, id, title string) error {
	if s.operations != nil {
		*s.operations = append(*s.operations, "session.rename")
	}
	if id != s.sess.ID {
		return session.ErrNotFound
	}
	if s.renamed != nil {
		s.renamed <- title
	}
	return s.renameErr
}

type fakeCheckpoints struct {
	snapshotted chan<- string
	operations  *[]string
	err         error
	started     chan<- struct{}
	release     <-chan struct{}
}

func (c fakeCheckpoints) Snapshot(_ context.Context, sessionID, cwd, runID string) error {
	if c.operations != nil {
		*c.operations = append(*c.operations, "checkpoint.snapshot")
	}
	if c.snapshotted != nil {
		c.snapshotted <- sessionID + ":" + cwd + ":" + runID
	}
	if c.started != nil {
		c.started <- struct{}{}
	}
	if c.release != nil {
		<-c.release
	}
	return c.err
}

type inlineTaskLauncher struct{}

func (inlineTaskLauncher) Start(ctx context.Context, task func(context.Context)) bool {
	task(ctx)
	return true
}
