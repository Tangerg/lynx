package runrecovery

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessionadmission"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/sqlite"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/itemfixture"
	runfixture "github.com/Tangerg/lynx/app/runtime/internal/testsupport/runfixture"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/sessionfixture"
	corechat "github.com/Tangerg/lynx/core/chat"
)

type alwaysResumable struct{}

func newTestRecovery(
	store runs.RecoveryStore,
	resumability runs.WaitingExecutionResumability,
) (*runs.Recovery, error) {
	return runs.NewRecovery(store, resumability, new(sessionadmission.Gate))
}

func (alwaysResumable) CanResumeWaitingExecution(
	context.Context,
	runs.WaitingContinuation,
) (bool, error) {
	return true, nil
}

type goalRunRecorderFunc func(context.Context, goal.RunRecord) error

func (record goalRunRecorderFunc) RecordRun(ctx context.Context, value goal.RunRecord) error {
	return record(ctx, value)
}

type childRunStartReservationsFunc func(context.Context, string) error

func (cleanup childRunStartReservationsFunc) DeleteSession(ctx context.Context, sessionID string) error {
	return cleanup(ctx, sessionID)
}

func TestRecoveryMarksClaimedResumeLostAndRemovesItsHiddenRecord(t *testing.T) {
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	createdAt := time.Date(2026, 8, 1, 2, 0, 0, 0, time.UTC)
	sessionStore := sqlite.NewSessionStore(db)
	if err := sessionStore.Insert(ctx, sessionfixture.MustRestore(session.Snapshot{
		ID: "session_claim", CWD: "/workspace", StartedAt: createdAt, UpdatedAt: createdAt,
	})); err != nil {
		t.Fatalf("seed Session: %v", err)
	}
	runStore := sqlite.NewRunStore(db)
	capabilities := run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Question}}
	if err := runStore.Admit(ctx, run.Draft{
		RunID: "run_claim", SessionID: "session_claim", SegmentID: "segment_claim",
		Capabilities: capabilities, CreatedAt: createdAt,
	}); err != nil {
		t.Fatalf("Admit: %v", err)
	}
	request := transcript.Interrupt{
		ItemID: "item_claim", ItemOccurredAt: createdAt.Add(time.Second),
		RunID: "run_claim", Kind: interrupt.Question,
		Question: &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?"}}},
	}
	waiting := runfixture.MustRestore(run.Snapshot{ID: "run_claim", SessionID: "session_claim", State: run.Waiting,
		Capabilities: capabilities,
		CreatedAt:    createdAt, UpdatedAt: createdAt.Add(time.Second),
		MessageMark: run.UnknownMessageMark})

	if err := runStore.Suspend(ctx, waiting); err != nil {
		t.Fatalf("Suspend: %v", err)
	}
	pending := runs.Pending{
		RootRunID: "run_claim", SessionID: "session_claim", ExecutorID: "execution_claim",
		Capabilities: capabilities, Interrupts: []transcript.Interrupt{request},
		Bindings: []runs.InterruptBinding{{
			InterruptItemID: request.ItemID, MemberID: "member_claim", RequestID: "request_claim",
		}},
		Continuations: []runs.Continuation{{
			RunID: "run_claim", MemberID: "member_claim", RunCreatedAt: createdAt,
		}},
		CreatedAt: createdAt.Add(time.Second),
	}
	interruptStore := persistence.NewInterruptStore(sqlite.NewInterruptStore(db))
	if err := interruptStore.Open(ctx, pending); err != nil {
		t.Fatalf("Open Pending: %v", err)
	}
	answers := []runs.InterruptAnswer{{
		InterruptItemID: request.ItemID, MemberID: "member_claim", RequestID: "request_claim",
		Resolution: interrupt.Resolution{Answers: [][]string{{"continue"}}},
	}}
	if _, found, err := interruptStore.ClaimResume(
		ctx, pending.SessionID, pending.RootRunID, answers, createdAt.Add(2*time.Second),
	); err != nil || !found {
		t.Fatalf("ClaimResume: found=%t err=%v", found, err)
	}
	if _, found, err := interruptStore.Get(ctx, pending.RootRunID); err != nil || found {
		t.Fatalf("open Pending after claim = found:%t err:%v", found, err)
	}

	messageStore := sqlite.NewMessageStore(db)
	if err := messageStore.Write(
		ctx,
		pending.SessionID,
		corechat.NewUserMessage(corechat.NewTextPart("ask me")),
		corechat.NewAssistantMessage(corechat.NewToolCallPart(corechat.ToolCall{
			ID: "provider_call_claim", Name: "ask_user", Arguments: "{}",
		})),
	); err != nil {
		t.Fatalf("seed conversation: %v", err)
	}
	checkpointStore := persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db))
	store, err := New(Config{
		Sessions: sessionStore, Runs: runStore, Interrupts: interruptStore,
		Transcript: sqlite.NewTranscriptStore(db), Messages: messageStore,
		GoalRuns: sqlite.NewGoalStore(db), ExecutorCheckpoints: checkpointStore,
		ModelInvocations: sqlite.NewModelInvocationStore(db),
		ToolInvocations:  sqlite.NewToolInvocationStore(db),
		ChildRunStarts:   sqlite.NewChildRunStartReservationStore(db),
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlite.RunInTx(ctx, db, fn)
		},
	})
	if err != nil {
		t.Fatalf("New persistence: %v", err)
	}
	checkpointProbes := 0
	recovery, err := newTestRecovery(store, waitingExecutionResumabilityFunc(func(
		context.Context,
		runs.WaitingContinuation,
	) (bool, error) {
		checkpointProbes++
		return true, nil
	}))
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}
	recovered, err := recovery.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if recovered != 1 || checkpointProbes != 0 {
		t.Fatalf("recovered=%d checkpoint probes=%d, want 1/0", recovered, checkpointProbes)
	}
	stored, found, err := runStore.Run(ctx, pending.RootRunID)
	failure, failed := stored.Failure()
	if err != nil || !found || stored.State() != run.Failed ||
		!failed || failure.Kind != run.FailureLost || stored.MessageMark() != 3 {
		t.Fatalf("recovered Run = found:%t value:%+v err:%v", found, stored, err)
	}
	messages, err := messageStore.Read(ctx, pending.SessionID)
	if err != nil || len(messages) != 3 || messages[2].Role != corechat.RoleTool ||
		len(messages[2].Parts) != 1 || messages[2].Parts[0].ToolResult == nil ||
		messages[2].Parts[0].ToolResult.ID != "provider_call_claim" ||
		!messages[2].Parts[0].ToolResult.IsError {
		t.Fatalf("recovered conversation = %#v, %v", messages, err)
	}
	var remaining int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM interrupts WHERE root_run_id = ?`, pending.RootRunID,
	).Scan(&remaining); err != nil {
		t.Fatalf("count interrupt rows: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("hidden resuming rows = %d, want none", remaining)
	}
}

// TestRecoveryCleanupIsScopedToClaimedSessions proves one Runtime never sweeps
// another live Runtime's callback or checkpoint facts from the shared DB.
func TestRecoveryCleanupIsScopedToClaimedSessions(t *testing.T) {
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	childStarts := sqlite.NewChildRunStartReservationStore(db)
	if err := childStarts.Reserve(ctx, sqlite.ChildRunStartReservationRecord{
		MemberID: "member_abandoned", SessionID: "session_abandoned",
		Payload: []byte(`{"run":"child_abandoned"}`), CreatedAt: time.Unix(1, 0).UTC(),
	}); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	checkpointStore := persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db))
	checkpoint := runs.ExecutorCheckpoint{
		RootMemberID: "member_orphan", Payload: []byte(`{"opaque":true}`), BuildID: "build",
		Scope: runs.ExecutionScope{SessionID: "session_abandoned"},
	}
	if err := checkpointStore.SaveCheckpoint(ctx, checkpoint); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}
	cleanupFailure := errors.New("child-start cleanup failed")
	failingStore, err := New(Config{
		Sessions: sqlite.NewSessionStore(db), Runs: sqlite.NewRunStore(db),
		Interrupts: persistence.NewInterruptStore(sqlite.NewInterruptStore(db)),
		Transcript: sqlite.NewTranscriptStore(db), Messages: sqlite.NewMessageStore(db),
		ExecutorCheckpoints: checkpointStore,
		ModelInvocations:    sqlite.NewModelInvocationStore(db),
		ToolInvocations:     sqlite.NewToolInvocationStore(db),
		ChildRunStarts: childRunStartReservationsFunc(func(context.Context, string) error {
			return cleanupFailure
		}),
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlite.RunInTx(ctx, db, fn)
		},
	})
	if err != nil {
		t.Fatalf("New failing persistence: %v", err)
	}
	commit := runs.RecoveryCommit{
		RecoveredSessionIDs:        []string{"session_abandoned"},
		DeleteCheckpointSessionIDs: []string{"session_abandoned"},
	}
	if err := failingStore.CommitRecovery(ctx, commit); !errors.Is(err, cleanupFailure) {
		t.Fatalf("failed CommitRecovery = %v, want %v", err, cleanupFailure)
	}
	if _, err := checkpointStore.LoadCheckpoint(ctx, checkpoint.RootMemberID); err != nil {
		t.Fatalf("cleanup failure did not roll back preceding checkpoint cleanup: %v", err)
	}

	store, err := New(Config{
		Sessions: sqlite.NewSessionStore(db), Runs: sqlite.NewRunStore(db),
		Interrupts: persistence.NewInterruptStore(sqlite.NewInterruptStore(db)),
		Transcript: sqlite.NewTranscriptStore(db), Messages: sqlite.NewMessageStore(db),
		ExecutorCheckpoints: checkpointStore,
		ModelInvocations:    sqlite.NewModelInvocationStore(db),
		ToolInvocations:     sqlite.NewToolInvocationStore(db),
		ChildRunStarts:      childStarts,
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlite.RunInTx(ctx, db, fn)
		},
	})
	if err != nil {
		t.Fatalf("New persistence: %v", err)
	}
	if err := store.CommitRecovery(ctx, commit); err != nil {
		t.Fatalf("CommitRecovery: %v", err)
	}
	var remaining int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM child_run_start_reservations`,
	).Scan(&remaining); err != nil {
		t.Fatalf("count child-start reservations: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("child-start reservations after boot recovery = %d, want 0", remaining)
	}
	if _, err := checkpointStore.LoadCheckpoint(ctx, checkpoint.RootMemberID); !errors.Is(err, runs.ErrExecutorCheckpointNotFound) {
		t.Fatalf("orphan checkpoint after successful recovery = %v, want not found", err)
	}
}

type waitingExecutionResumabilityFunc func(context.Context, runs.WaitingContinuation) (bool, error)

func (probe waitingExecutionResumabilityFunc) CanResumeWaitingExecution(
	ctx context.Context,
	continuation runs.WaitingContinuation,
) (bool, error) {
	return probe(ctx, continuation)
}

// TestRecoveryRepairsWholeDurableLifecycle proves
// terminal_run_explains_how_it_ended at the runs.recovery boundary.
func TestRecoveryRepairsWholeDurableLifecycle(t *testing.T) {
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	createdAt := time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC)
	runStore := sqlite.NewRunStore(db)
	sessionStore := sqlite.NewSessionStore(db)
	if err := sessionStore.Insert(ctx, sessionfixture.MustRestore(session.Snapshot{
		ID: "session", CWD: "/workspace", StartedAt: createdAt, UpdatedAt: createdAt,
	})); err != nil {
		t.Fatalf("seed Session: %v", err)
	}
	interruptStore := persistence.NewInterruptStore(sqlite.NewInterruptStore(db))
	transcriptStore := sqlite.NewTranscriptStore(db)
	messageStore := sqlite.NewMessageStore(db)
	goalStore := sqlite.NewGoalStore(db)
	checkpointStore := persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db))
	modelInvocations := sqlite.NewModelInvocationStore(db)
	toolInvocations := sqlite.NewToolInvocationStore(db)
	goalValue, err := goal.New("session", "finish recovery", modelref.Selection{}, goal.Budget{}, run.Capabilities{}, "lease_recovery", createdAt)
	if err != nil {
		t.Fatalf("New Goal: %v", err)
	}
	if _, applied, err := goalStore.Save(ctx, goalValue, goal.Version{}); err != nil || !applied {
		t.Fatalf("Save Goal: applied=%t err=%v", applied, err)
	}
	if err := runStore.Admit(ctx, run.Draft{
		RunID: "run_lost", SessionID: "session", SegmentID: "segment", GoalIncarnationID: goalValue.IncarnationID, CreatedAt: createdAt,
	}); err != nil {
		t.Fatalf("Admit: %v", err)
	}
	item := itemfixture.MustRestore(itemfixture.Input{
		ID: "item_running", SessionID: "session", RunID: "run_lost",
		Kind: transcript.QuestionItem, OccurredAt: createdAt,
		Question: &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?"}}},
	})
	if err := transcriptStore.AppendItem(ctx, item); err != nil {
		t.Fatalf("AppendItem: %v", err)
	}
	toolItem := itemfixture.MustRestore(itemfixture.Input{
		ID: "item_tool_running", SessionID: "session", RunID: "run_lost",
		Status: transcript.ItemRunning, Kind: transcript.ToolCall,
		Tool: &transcript.ToolInvocation{Name: "long_running_tool"}, OccurredAt: createdAt.Add(time.Second),
	})
	if err := transcriptStore.AppendItem(ctx, toolItem); err != nil {
		t.Fatalf("Append Tool Item: %v", err)
	}
	modelStartedAt := createdAt.Add(2 * time.Second)
	if err := modelInvocations.StartModelInvocation(
		ctx, "session", "run_lost", "segment", "model_call_lost", modelStartedAt,
	); err != nil {
		t.Fatalf("start model invocation: %v", err)
	}
	toolStartedAt := createdAt.Add(3 * time.Second)
	if err := toolInvocations.StartToolInvocation(
		ctx, "session", "run_lost", "segment", "tool_call_lost", toolItem.ID(), toolStartedAt,
	); err != nil {
		t.Fatalf("start Tool invocation: %v", err)
	}
	checkpoint := runs.ExecutorCheckpoint{
		RootMemberID: "orphan_checkpoint",
		Payload:      []byte(`{"opaque":true}`),
		BuildID:      "build",
		Scope:        runs.ExecutionScope{SessionID: "session"},
	}
	if err := checkpointStore.SaveCheckpoint(ctx, checkpoint); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}
	if err := messageStore.Write(
		ctx,
		"session",
		corechat.NewUserMessage(corechat.NewTextPart("recover this")),
		corechat.NewAssistantMessage(corechat.NewToolCallPart(corechat.ToolCall{
			ID: "provider_call_lost", Name: "ask_user", Arguments: "{}",
		})),
	); err != nil {
		t.Fatalf("seed conversation: %v", err)
	}
	rollbackFailure := errors.New("record recovered Goal Run")
	failingPersistence, err := New(Config{
		Sessions:   sessionStore,
		Runs:       runStore,
		Interrupts: interruptStore,
		Transcript: transcriptStore,
		Messages:   messageStore,
		GoalRuns: goalRunRecorderFunc(func(context.Context, goal.RunRecord) error {
			return rollbackFailure
		}),
		ExecutorCheckpoints: checkpointStore,
		ModelInvocations:    modelInvocations,
		ToolInvocations:     toolInvocations,
		ChildRunStarts:      sqlite.NewChildRunStartReservationStore(db),
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlite.RunInTx(ctx, db, fn)
		},
	})
	if err != nil {
		t.Fatalf("New failing persistence: %v", err)
	}
	failingRecovery, err := newTestRecovery(failingPersistence, alwaysResumable{})
	if err != nil {
		t.Fatalf("New failing Recovery: %v", err)
	}
	if _, err := failingRecovery.Reconcile(ctx); !errors.Is(err, rollbackFailure) {
		t.Fatalf("failed Reconcile error = %v, want %v", err, rollbackFailure)
	}
	afterRollback, err := messageStore.Read(ctx, "session")
	if err != nil || len(afterRollback) != 2 {
		t.Fatalf("conversation after rollback = %#v, %v", afterRollback, err)
	}
	activeAfterRollback, found, err := runStore.Run(ctx, "run_lost")
	if err != nil || !found || activeAfterRollback.State() != run.Running {
		t.Fatalf("Run after rollback = found:%t value:%+v err:%v", found, activeAfterRollback, err)
	}
	if _, err := checkpointStore.LoadCheckpoint(ctx, checkpoint.RootMemberID); err != nil {
		t.Fatalf("checkpoint after rollback: %v", err)
	}
	var modelState, toolState string
	if err := db.QueryRowContext(ctx,
		`SELECT state FROM model_invocations WHERE call_id = ?`, "model_call_lost",
	).Scan(&modelState); err != nil || modelState != "started" {
		t.Fatalf("model invocation after rollback = %q, %v, want started", modelState, err)
	}
	if err := db.QueryRowContext(ctx,
		`SELECT state FROM tool_invocations WHERE call_id = ? AND segment_id = ?`,
		"tool_call_lost", "segment",
	).Scan(&toolState); err != nil || toolState != "started" {
		t.Fatalf("Tool invocation after rollback = %q, %v, want started", toolState, err)
	}
	persistence, err := New(Config{
		Sessions:            sessionStore,
		Runs:                runStore,
		Interrupts:          interruptStore,
		Transcript:          transcriptStore,
		Messages:            messageStore,
		GoalRuns:            goalStore,
		ExecutorCheckpoints: checkpointStore,
		ModelInvocations:    modelInvocations,
		ToolInvocations:     toolInvocations,
		ChildRunStarts:      sqlite.NewChildRunStartReservationStore(db),
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlite.RunInTx(ctx, db, fn)
		},
	})
	if err != nil {
		t.Fatalf("New persistence: %v", err)
	}
	recovery, err := newTestRecovery(persistence, alwaysResumable{})
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}

	recovered, err := recovery.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if recovered != 1 {
		t.Fatalf("recovered Runs = %d, want 1", recovered)
	}
	stored, found, err := runStore.Run(ctx, "run_lost")
	failure, failed := stored.Failure()
	if err != nil || !found || stored.State() != run.Failed ||
		!failed || failure.Kind != run.FailureLost || stored.MessageMark() != 3 {
		t.Fatalf("recovered Run = found:%t value:%+v err:%v", found, stored, err)
	}
	recoveredMessages, err := messageStore.Read(ctx, "session")
	if err != nil || len(recoveredMessages) != 3 || recoveredMessages[2].Role != corechat.RoleTool ||
		len(recoveredMessages[2].Parts) != 1 || recoveredMessages[2].Parts[0].ToolResult == nil ||
		recoveredMessages[2].Parts[0].ToolResult.ID != "provider_call_lost" ||
		!recoveredMessages[2].Parts[0].ToolResult.IsError {
		t.Fatalf("recovered conversation = %#v, %v", recoveredMessages, err)
	}
	storedItem, found, err := transcriptStore.Item(ctx, item.ID())
	if err != nil || !found || storedItem.Status() != transcript.ItemCompleted ||
		!reflect.DeepEqual(storedItem.Snapshot(), item.Snapshot()) {
		t.Fatalf("recovered Item = found:%t value:%+v err:%v", found, storedItem, err)
	}
	recoveredToolItem, found, err := transcriptStore.Item(ctx, toolItem.ID())
	if err != nil || !found || recoveredToolItem.Status() != transcript.ItemIncomplete {
		t.Fatalf("recovered Tool Item = found:%t value:%+v err:%v", found, recoveredToolItem, err)
	}
	var modelFinishedAt, toolFinishedAt int64
	if err := db.QueryRowContext(ctx,
		`SELECT state, finished_at FROM model_invocations WHERE call_id = ?`, "model_call_lost",
	).Scan(&modelState, &modelFinishedAt); err != nil || modelState != "unknown" || modelFinishedAt == 0 {
		t.Fatalf("recovered model invocation = state:%q finished:%d err:%v", modelState, modelFinishedAt, err)
	}
	if err := db.QueryRowContext(ctx,
		`SELECT state, finished_at FROM tool_invocations WHERE call_id = ? AND segment_id = ?`,
		"tool_call_lost", "segment",
	).Scan(&toolState, &toolFinishedAt); err != nil || toolState != "incomplete" || toolFinishedAt == 0 {
		t.Fatalf("recovered Tool invocation = state:%q finished:%d err:%v", toolState, toolFinishedAt, err)
	}
	if _, err := checkpointStore.LoadCheckpoint(ctx, checkpoint.RootMemberID); !errors.Is(err, runs.ErrExecutorCheckpointNotFound) {
		t.Fatalf("orphan checkpoint after recovery = %v", err)
	}
	storedGoal, found, err := goalStore.Get(ctx, goalValue.SessionID)
	if err != nil || !found || storedGoal.Used.Runs != 1 || storedGoal.Status != goal.StatusPaused || storedGoal.Reason.Code != goal.ReasonRunNotCompleted {
		t.Fatalf("Goal after recovery = found:%t value:%+v err:%v", found, storedGoal, err)
	}
}

// TestRecoveryRejectsPartialParkWithoutMutatingIt proves
// parked_tree_has_exactly_one_open_interrupt_set at the runs.recovery boundary.
func TestRecoveryRejectsPartialParkWithoutMutatingIt(t *testing.T) {
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	createdAt := time.Date(2026, 8, 1, 4, 0, 0, 0, time.UTC)
	runStore := sqlite.NewRunStore(db)
	sessionStore := sqlite.NewSessionStore(db)
	if err := sessionStore.Insert(ctx, sessionfixture.MustRestore(session.Snapshot{
		ID: "session", CWD: "/workspace", StartedAt: createdAt, UpdatedAt: createdAt,
	})); err != nil {
		t.Fatalf("seed Session: %v", err)
	}
	interruptStore := persistence.NewInterruptStore(sqlite.NewInterruptStore(db))
	transcriptStore := sqlite.NewTranscriptStore(db)
	checkpointStore := persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db))
	if err := runStore.Admit(ctx, run.Draft{
		RunID: "run_partial", SessionID: "session", SegmentID: "segment", CreatedAt: createdAt,
		Capabilities: run.Capabilities{
			InterruptKinds: []interrupt.Kind{interrupt.Question},
		},
	}); err != nil {
		t.Fatalf("Admit: %v", err)
	}
	question := &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?"}}}
	pendingInterrupt := transcript.Interrupt{
		ItemID: "item_missing", ItemOccurredAt: createdAt.Add(time.Second),
		RunID: "run_partial", Kind: interrupt.Question, Question: question,
	}
	if err := runStore.Suspend(ctx, runfixture.MustRestore(run.Snapshot{ID: "run_partial", SessionID: "session", State: run.Waiting,
		Capabilities: run.Capabilities{
			InterruptKinds: []interrupt.Kind{interrupt.Question},
		},
		CreatedAt: createdAt,
		UpdatedAt: createdAt.Add(time.Second), MessageMark: run.UnknownMessageMark}),
	); err != nil {
		t.Fatalf("Suspend: %v", err)
	}
	pending := runs.Pending{
		RootRunID: "run_partial", SessionID: "session", ExecutorID: "turn_partial",
		Capabilities: run.Capabilities{
			InterruptKinds: []interrupt.Kind{interrupt.Question},
		},
		Interrupts: []transcript.Interrupt{pendingInterrupt},
		Bindings: []runs.InterruptBinding{{
			InterruptItemID: pendingInterrupt.ItemID, MemberID: "member_root", RequestID: "request_root",
		}},
		Continuations: []runs.Continuation{{
			RunID: "run_partial", MemberID: "member_root", RunCreatedAt: createdAt,
		}},
		CreatedAt: createdAt.Add(time.Second),
	}
	if err := interruptStore.Open(ctx, pending); err != nil {
		t.Fatalf("Open Pending: %v", err)
	}
	checkpoint := runs.ExecutorCheckpoint{
		RootMemberID: "member_root", Payload: []byte(`{"opaque":true}`), BuildID: "build",
		Scope: runs.ExecutionScope{SessionID: "session"},
	}
	if err := checkpointStore.SaveCheckpoint(ctx, checkpoint); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}
	persistence, err := New(Config{
		Sessions: sessionStore, Runs: runStore, Interrupts: interruptStore, Transcript: transcriptStore,
		Messages: sqlite.NewMessageStore(db), GoalRuns: sqlite.NewGoalStore(db), ExecutorCheckpoints: checkpointStore,
		ModelInvocations: sqlite.NewModelInvocationStore(db), ToolInvocations: sqlite.NewToolInvocationStore(db),
		ChildRunStarts: sqlite.NewChildRunStartReservationStore(db),
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlite.RunInTx(ctx, db, fn)
		},
	})
	if err != nil {
		t.Fatalf("New persistence: %v", err)
	}
	recovery, err := newTestRecovery(persistence, alwaysResumable{})
	if err != nil {
		t.Fatalf("NewRecovery: %v", err)
	}

	if _, err := recovery.Reconcile(ctx); err == nil {
		t.Fatal("Reconcile accepted a Pending whose interrupt Item is absent")
	}
	if _, found, err := interruptStore.Get(ctx, pending.RootRunID); err != nil || !found {
		t.Fatalf("Pending after rejection = found:%t err:%v, want preserved", found, err)
	}
	stored, found, err := runStore.Run(ctx, pending.RootRunID)
	if err != nil || !found || stored.State() != run.Waiting {
		t.Fatalf("Run after rejection = found:%t value:%+v err:%v", found, stored, err)
	}
	if _, err := checkpointStore.LoadCheckpoint(ctx, checkpoint.RootMemberID); err != nil {
		t.Fatalf("checkpoint after rejection: %v", err)
	}
}
