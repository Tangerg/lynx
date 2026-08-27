package runrecovery

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/scope/app/runtime/internal/application/invalidation"
	"github.com/Tangerg/scope/app/runtime/internal/application/runs"
	"github.com/Tangerg/scope/app/runtime/internal/application/sessionadmission"
	"github.com/Tangerg/scope/app/runtime/internal/domain/goal"
	"github.com/Tangerg/scope/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/scope/app/runtime/internal/domain/run"
	"github.com/Tangerg/scope/app/runtime/internal/domain/session"
	"github.com/Tangerg/scope/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/scope/app/runtime/internal/infra/sqlite"
	"github.com/Tangerg/scope/app/runtime/internal/testsupport/itemfixture"
	runfixture "github.com/Tangerg/scope/app/runtime/internal/testsupport/runfixture"
	"github.com/Tangerg/scope/app/runtime/internal/testsupport/sessionfixture"
	corechat "github.com/Tangerg/scope/core/chat"
)

type alwaysResumable struct{}

func newTestRecovery(
	store runs.RecoveryStore,
	resumability runs.WaitingExecutionResumability,
) (*runs.Recovery, error) {
	return runs.NewRecovery(store, resumability, new(sessionadmission.Gate), nil)
}

func (alwaysResumable) CanResumeWaitingExecution(
	context.Context,
	runs.WaitingContinuation,
) (bool, error) {
	return true, nil
}

type goalRunRecorderFunc func(context.Context, goal.RunRecord) error

func (g goalRunRecorderFunc) RecordRun(ctx context.Context, value goal.RunRecord) error {
	return g(ctx, value)
}

type childRunStartReservationsFunc func(context.Context, string) error

func (c childRunStartReservationsFunc) DeleteSession(ctx context.Context, sessionID string) error {
	return c(ctx, sessionID)
}

func TestRecoveryMarksClaimedResumeLostAndRemovesItsHiddenRecord(t *testing.T) {
	for _, test := range []struct {
		name             string
		openingCommitted bool
	}{
		{name: "answer claimed before opening"},
		{name: "opening committed before activation", openingCommitted: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			testRecoveryMarksClaimedResumeLost(t, test.openingCommitted)
		})
	}
}

func testRecoveryMarksClaimedResumeLost(t *testing.T, openingCommitted bool) {
	t.Helper()
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	createdAt := time.Date(2026, 8, 1, 2, 0, 0, 0, time.UTC)
	sessionStore := sqlite.NewSessionStore(db)
	if insertErr := sessionStore.Insert(ctx, sessionfixture.MustRestore(session.Snapshot{
		ID: "session_claim", Workspace: sessionfixture.MustWorkspace("/workspace"), StartedAt: createdAt, UpdatedAt: createdAt,
	})); insertErr != nil {
		t.Fatalf("seed Session: %v", insertErr)
	}
	runStore := sqlite.NewRunStore(db)
	capabilities := run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Question}}
	if admitErr := runStore.Admit(ctx, run.Draft{
		RunID: "run_claim", SessionID: "session_claim", SegmentID: "segment_claim",
		Capabilities: capabilities, CreatedAt: createdAt,
	}); admitErr != nil {
		t.Fatalf("Admit: %v", admitErr)
	}
	request := transcript.Interrupt{
		ItemID: "item_claim", ItemOccurredAt: createdAt.Add(time.Second),
		RunID: "run_claim", Kind: interrupt.Question,
		Question: &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?", Kind: transcript.QuestionText}}},
	}
	waiting := runfixture.MustRestore(run.Snapshot{ID: "run_claim", SessionID: "session_claim", State: run.Waiting,
		Capabilities: capabilities,
		CreatedAt:    createdAt, UpdatedAt: createdAt.Add(time.Second),
		MessageMark: run.UnknownMessageMark})

	if suspendErr := runStore.Suspend(ctx, waiting); suspendErr != nil {
		t.Fatalf("Suspend: %v", suspendErr)
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
	if openErr := interruptStore.Open(ctx, pending); openErr != nil {
		t.Fatalf("Open Pending: %v", openErr)
	}
	answers := []runs.InterruptAnswer{{
		InterruptItemID: request.ItemID, MemberID: "member_claim", RequestID: "request_claim",
		Resolution: interrupt.Resolution{Answers: [][]string{{"continue"}}},
	}}
	if _, found, claimResumeErr := interruptStore.ClaimResume(
		ctx, pending.SessionID, pending.RootRunID, answers, createdAt.Add(2*time.Second),
	); claimResumeErr != nil || !found {
		t.Fatalf("ClaimResume: found=%t err=%v", found, claimResumeErr)
	}
	if _, found, getErr := interruptStore.Get(ctx, pending.RootRunID); getErr != nil || found {
		t.Fatalf("open Pending after claim = found:%t err:%v", found, getErr)
	}
	if openingCommitted {
		if resumeErr := runStore.Resume(ctx, pending.SessionID, run.ResumeDraft{
			RunID: pending.RootRunID, SegmentID: "segment_claim_resumed",
		}, createdAt.Add(3*time.Second)); resumeErr != nil {
			t.Fatalf("commit continuation opening: %v", resumeErr)
		}
		opened, found, runErr := runStore.Run(ctx, pending.RootRunID)
		if runErr != nil || !found || opened.State() != run.Running ||
			opened.ActiveSegmentID() != "segment_claim_resumed" {
			t.Fatalf("opened continuation = found:%t value:%+v err:%v", found, opened, runErr)
		}
	}
	transcriptStore := sqlite.NewTranscriptStore(db)
	questionItem, err := transcript.NewQuestion(transcript.ItemIdentity{
		SessionID: pending.SessionID, RunID: request.RunID, ItemID: request.ItemID,
		OccurredAt: request.ItemOccurredAt,
	}, *request.Question)
	if err != nil {
		t.Fatalf("new Question Item: %v", err)
	}
	answeredQuestion, err := questionItem.AnswerQuestion([][]string{{"continue"}})
	if err != nil {
		t.Fatalf("answer Question Item: %v", err)
	}
	if appendItemErr := transcriptStore.AppendItem(ctx, answeredQuestion); appendItemErr != nil {
		t.Fatalf("seed accepted Question Item: %v", appendItemErr)
	}

	messageStore := sqlite.NewMessageStore(db)
	if writeErr := messageStore.Write(
		ctx,
		pending.SessionID,
		corechat.NewUserMessage(corechat.NewTextPart("ask me")),
		corechat.NewAssistantMessage(corechat.NewToolCallPart(corechat.ToolCall{
			ID: "provider_call_claim", Name: "ask_user", Arguments: "{}",
		})),
	); writeErr != nil {
		t.Fatalf("seed conversation: %v", writeErr)
	}
	checkpointStore := persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db))
	store, err := New(Config{
		Sessions: sessionStore, Runs: runStore, Interrupts: interruptStore,
		Transcript: transcriptStore, Messages: messageStore,
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
	var notices []invalidation.Notice
	recovery, err := runs.NewRecovery(store, waitingExecutionResumabilityFunc(func(
		context.Context,
		runs.WaitingContinuation,
	) (bool, error) {
		checkpointProbes++
		return true, nil
	}), new(sessionadmission.Gate), func(notice invalidation.Notice) {
		notices = append(notices, notice)
	})
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
	if scanErr := db.QueryRowContext(ctx,
		`SELECT count(*) FROM interrupts WHERE root_run_id = ?`, pending.RootRunID,
	).Scan(&remaining); scanErr != nil {
		t.Fatalf("count interrupt rows: %v", scanErr)
	}
	if remaining != 0 {
		t.Fatalf("hidden resuming rows = %d, want none", remaining)
	}
	storedQuestion, found, err := transcriptStore.Item(ctx, request.ItemID)
	if err != nil || !found {
		t.Fatalf("accepted Question after recovery = found:%t err:%v", found, err)
	}
	question, ok := storedQuestion.Question()
	if !ok || !reflect.DeepEqual(question.Answers, [][]string{{"continue"}}) {
		t.Fatalf("accepted Question after recovery = %+v, want durable answer", storedQuestion)
	}
	wantNotices := []invalidation.Notice{
		invalidation.InSession(invalidation.Runs, pending.SessionID, pending.RootRunID),
		invalidation.InSession(invalidation.Interrupts, pending.SessionID, pending.RootRunID),
		invalidation.InSession(invalidation.Sessions, pending.SessionID),
	}
	if !reflect.DeepEqual(notices, wantNotices) {
		t.Fatalf("recovery notices = %+v, want %+v", notices, wantNotices)
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
	if reserveErr := childStarts.Reserve(ctx, sqlite.ChildRunStartReservationRecord{
		MemberID: "member_abandoned", SessionID: "session_abandoned",
		Payload: []byte(`{"run":"child_abandoned"}`), CreatedAt: time.Unix(1, 0).UTC(),
	}); reserveErr != nil {
		t.Fatalf("Reserve: %v", reserveErr)
	}
	checkpointStore := persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db))
	checkpoint := runs.ExecutorCheckpoint{
		RootMemberID: "member_orphan", Payload: []byte(`{"opaque":true}`), BuildID: "build",
		Scope: runs.ExecutionScope{SessionID: "session_abandoned"},
	}
	if saveCheckpointErr := checkpointStore.SaveCheckpoint(ctx, checkpoint); saveCheckpointErr != nil {
		t.Fatalf("SaveCheckpoint: %v", saveCheckpointErr)
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
	if commitRecoveryErr := failingStore.CommitRecovery(ctx, commit); !errors.Is(commitRecoveryErr, cleanupFailure) {
		t.Fatalf("failed CommitRecovery = %v, want %v", commitRecoveryErr, cleanupFailure)
	}
	if _, loadCheckpointErr := checkpointStore.LoadCheckpoint(ctx, checkpoint.RootMemberID); loadCheckpointErr != nil {
		t.Fatalf("cleanup failure did not roll back preceding checkpoint cleanup: %v", loadCheckpointErr)
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

func (w waitingExecutionResumabilityFunc) CanResumeWaitingExecution(
	ctx context.Context,
	continuation runs.WaitingContinuation,
) (bool, error) {
	return w(ctx, continuation)
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
	if insertErr := sessionStore.Insert(ctx, sessionfixture.MustRestore(session.Snapshot{
		ID: "session", Workspace: sessionfixture.MustWorkspace("/workspace"), StartedAt: createdAt, UpdatedAt: createdAt,
	})); insertErr != nil {
		t.Fatalf("seed Session: %v", insertErr)
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
	if _, applied, saveErr := goalStore.Save(ctx, goalValue, goal.Version{}); saveErr != nil || !applied {
		t.Fatalf("Save Goal: applied=%t err=%v", applied, saveErr)
	}
	if admitErr := runStore.Admit(ctx, run.Draft{
		RunID: "run_lost", SessionID: "session", SegmentID: "segment", GoalIncarnationID: goalValue.IncarnationID, CreatedAt: createdAt,
	}); admitErr != nil {
		t.Fatalf("Admit: %v", admitErr)
	}
	item := itemfixture.MustRestore(itemfixture.Input{
		ID: "item_running", SessionID: "session", RunID: "run_lost",
		Kind: transcript.QuestionItem, OccurredAt: createdAt,
		Question: &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?", Kind: transcript.QuestionText}}},
	})
	if appendItemErr := transcriptStore.AppendItem(ctx, item); appendItemErr != nil {
		t.Fatalf("AppendItem: %v", appendItemErr)
	}
	toolItem := itemfixture.MustRestore(itemfixture.Input{
		ID: "item_tool_running", SessionID: "session", RunID: "run_lost",
		Status: transcript.ItemRunning, Kind: transcript.ToolCall,
		Tool: &transcript.ToolInvocation{Name: "long_running_tool"}, OccurredAt: createdAt.Add(time.Second),
	})
	if appendItemErr := transcriptStore.AppendItem(ctx, toolItem); appendItemErr != nil {
		t.Fatalf("Append Tool Item: %v", appendItemErr)
	}
	modelStartedAt := createdAt.Add(2 * time.Second)
	if startModelInvocationErr := modelInvocations.StartModelInvocation(
		ctx, "session", "run_lost", "segment", "model_call_lost", modelStartedAt,
	); startModelInvocationErr != nil {
		t.Fatalf("start model invocation: %v", startModelInvocationErr)
	}
	toolStartedAt := createdAt.Add(3 * time.Second)
	if startToolInvocationErr := toolInvocations.StartToolInvocation(
		ctx, "session", "run_lost", "segment", "tool_call_lost", toolItem.ID(), toolStartedAt,
	); startToolInvocationErr != nil {
		t.Fatalf("start Tool invocation: %v", startToolInvocationErr)
	}
	checkpoint := runs.ExecutorCheckpoint{
		RootMemberID: "orphan_checkpoint",
		Payload:      []byte(`{"opaque":true}`),
		BuildID:      "build",
		Scope:        runs.ExecutionScope{SessionID: "session"},
	}
	if saveCheckpointErr := checkpointStore.SaveCheckpoint(ctx, checkpoint); saveCheckpointErr != nil {
		t.Fatalf("SaveCheckpoint: %v", saveCheckpointErr)
	}
	if writeErr := messageStore.Write(
		ctx,
		"session",
		corechat.NewUserMessage(corechat.NewTextPart("recover this")),
		corechat.NewAssistantMessage(corechat.NewToolCallPart(corechat.ToolCall{
			ID: "provider_call_lost", Name: "ask_user", Arguments: "{}",
		})),
	); writeErr != nil {
		t.Fatalf("seed conversation: %v", writeErr)
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
	if _, reconcileErr := failingRecovery.Reconcile(ctx); !errors.Is(reconcileErr, rollbackFailure) {
		t.Fatalf("failed Reconcile error = %v, want %v", reconcileErr, rollbackFailure)
	}
	afterRollback, err := messageStore.Read(ctx, "session")
	if err != nil || len(afterRollback) != 2 {
		t.Fatalf("conversation after rollback = %#v, %v", afterRollback, err)
	}
	activeAfterRollback, found, err := runStore.Run(ctx, "run_lost")
	if err != nil || !found || activeAfterRollback.State() != run.Running {
		t.Fatalf("Run after rollback = found:%t value:%+v err:%v", found, activeAfterRollback, err)
	}
	if _, loadCheckpointErr := checkpointStore.LoadCheckpoint(ctx, checkpoint.RootMemberID); loadCheckpointErr != nil {
		t.Fatalf("checkpoint after rollback: %v", loadCheckpointErr)
	}
	var modelState, toolState string
	if scanErr := db.QueryRowContext(ctx,
		`SELECT state FROM model_invocations WHERE call_id = ?`, "model_call_lost",
	).Scan(&modelState); scanErr != nil || modelState != "started" {
		t.Fatalf("model invocation after rollback = %q, %v, want started", modelState, scanErr)
	}
	if scanErr := db.QueryRowContext(ctx,
		`SELECT state FROM tool_invocations WHERE call_id = ? AND segment_id = ?`,
		"tool_call_lost", "segment",
	).Scan(&toolState); scanErr != nil || toolState != "started" {
		t.Fatalf("Tool invocation after rollback = %q, %v, want started", toolState, scanErr)
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
	if scanErr := db.QueryRowContext(ctx,
		`SELECT state, finished_at FROM model_invocations WHERE call_id = ?`, "model_call_lost",
	).Scan(&modelState, &modelFinishedAt); scanErr != nil || modelState != "unknown" || modelFinishedAt == 0 {
		t.Fatalf("recovered model invocation = state:%q finished:%d err:%v", modelState, modelFinishedAt, scanErr)
	}
	if scanErr := db.QueryRowContext(ctx,
		`SELECT state, finished_at FROM tool_invocations WHERE call_id = ? AND segment_id = ?`,
		"tool_call_lost", "segment",
	).Scan(&toolState, &toolFinishedAt); scanErr != nil || toolState != "incomplete" || toolFinishedAt == 0 {
		t.Fatalf("recovered Tool invocation = state:%q finished:%d err:%v", toolState, toolFinishedAt, scanErr)
	}
	if _, loadCheckpointErr := checkpointStore.LoadCheckpoint(ctx, checkpoint.RootMemberID); !errors.Is(loadCheckpointErr, runs.ErrExecutorCheckpointNotFound) {
		t.Fatalf("orphan checkpoint after recovery = %v", loadCheckpointErr)
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
	if insertErr := sessionStore.Insert(ctx, sessionfixture.MustRestore(session.Snapshot{
		ID: "session", Workspace: sessionfixture.MustWorkspace("/workspace"), StartedAt: createdAt, UpdatedAt: createdAt,
	})); insertErr != nil {
		t.Fatalf("seed Session: %v", insertErr)
	}
	interruptStore := persistence.NewInterruptStore(sqlite.NewInterruptStore(db))
	transcriptStore := sqlite.NewTranscriptStore(db)
	checkpointStore := persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db))
	if admitErr := runStore.Admit(ctx, run.Draft{
		RunID: "run_partial", SessionID: "session", SegmentID: "segment", CreatedAt: createdAt,
		Capabilities: run.Capabilities{
			InterruptKinds: []interrupt.Kind{interrupt.Question},
		},
	}); admitErr != nil {
		t.Fatalf("Admit: %v", admitErr)
	}
	question := &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?", Kind: transcript.QuestionText}}}
	pendingInterrupt := transcript.Interrupt{
		ItemID: "item_missing", ItemOccurredAt: createdAt.Add(time.Second),
		RunID: "run_partial", Kind: interrupt.Question, Question: question,
	}
	if suspendErr := runStore.Suspend(ctx, runfixture.MustRestore(run.Snapshot{ID: "run_partial", SessionID: "session", State: run.Waiting,
		Capabilities: run.Capabilities{
			InterruptKinds: []interrupt.Kind{interrupt.Question},
		},
		CreatedAt: createdAt,
		UpdatedAt: createdAt.Add(time.Second), MessageMark: run.UnknownMessageMark}),
	); suspendErr != nil {
		t.Fatalf("Suspend: %v", suspendErr)
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
	if openErr := interruptStore.Open(ctx, pending); openErr != nil {
		t.Fatalf("Open Pending: %v", openErr)
	}
	checkpoint := runs.ExecutorCheckpoint{
		RootMemberID: "member_root", Payload: []byte(`{"opaque":true}`), BuildID: "build",
		Scope: runs.ExecutionScope{SessionID: "session"},
	}
	if saveCheckpointErr := checkpointStore.SaveCheckpoint(ctx, checkpoint); saveCheckpointErr != nil {
		t.Fatalf("SaveCheckpoint: %v", saveCheckpointErr)
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

	if _, reconcileErr := recovery.Reconcile(ctx); reconcileErr == nil {
		t.Fatal("Reconcile accepted a Pending whose interrupt Item is absent")
	}
	if _, found, getErr := interruptStore.Get(ctx, pending.RootRunID); getErr != nil || !found {
		t.Fatalf("Pending after rejection = found:%t err:%v, want preserved", found, getErr)
	}
	stored, found, err := runStore.Run(ctx, pending.RootRunID)
	if err != nil || !found || stored.State() != run.Waiting {
		t.Fatalf("Run after rejection = found:%t value:%+v err:%v", found, stored, err)
	}
	if _, err := checkpointStore.LoadCheckpoint(ctx, checkpoint.RootMemberID); err != nil {
		t.Fatalf("checkpoint after rejection: %v", err)
	}
}
