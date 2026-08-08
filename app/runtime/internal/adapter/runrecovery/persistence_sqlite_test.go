package runrecovery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

type alwaysResumable struct{}

func (alwaysResumable) CanResumeCheckpoint(
	context.Context,
	runs.ExecutorCheckpointExpectation,
) (bool, error) {
	return true, nil
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
	if err := sessionStore.Restore(ctx, session.Session{
		ID: "session", CWD: "/workspace", StartedAt: createdAt, UpdatedAt: createdAt,
	}); err != nil {
		t.Fatalf("seed Session: %v", err)
	}
	interruptStore := persistence.NewInterruptStore(sqlite.NewInterruptStore(db))
	transcriptStore := sqlite.NewTranscriptStore(db)
	messageStore := sqlite.NewMessageStore(db)
	goalStore := sqlite.NewGoalStore(db)
	checkpointStore := persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db))
	goalValue, err := goal.New("session", "finish recovery", modelref.Selection{}, goal.Budget{}, "lease_recovery", createdAt)
	if err != nil {
		t.Fatalf("New Goal: %v", err)
	}
	if _, applied, err := goalStore.Save(ctx, goalValue, goal.Version{}); err != nil || !applied {
		t.Fatalf("Save Goal: applied=%t err=%v", applied, err)
	}
	if err := runStore.Admit(ctx, run.RunDraft{
		RunID: "run_lost", SessionID: "session", SegmentID: "segment", GoalLeaseID: goalValue.LeaseID, CreatedAt: createdAt,
	}); err != nil {
		t.Fatalf("Admit: %v", err)
	}
	item := transcript.Item{
		ID: "item_running", SessionID: "session", RunID: "run_lost",
		Kind: transcript.QuestionItem, Status: transcript.ItemRunning, OccurredAt: createdAt,
		Question: &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?"}}},
	}
	if err := transcriptStore.AppendItem(ctx, item); err != nil {
		t.Fatalf("AppendItem: %v", err)
	}
	checkpoint := runs.ExecutorCheckpoint{
		RootProcessID: "orphan_checkpoint",
		Payload:       []byte(`{"opaque":true}`),
		BuildID:       "build",
		Scope:         runs.ExecutionScope{SessionID: "session"},
	}
	if err := checkpointStore.SaveCheckpoint(ctx, checkpoint); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}
	persistence, err := New(Config{
		Sessions:            sessionStore,
		Runs:                runStore,
		Interrupts:          interruptStore,
		Transcript:          transcriptStore,
		Messages:            messageStore,
		GoalRuns:            goalStore,
		ExecutorCheckpoints: checkpointStore,
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlite.RunInTx(ctx, db, fn)
		},
	})
	if err != nil {
		t.Fatalf("New persistence: %v", err)
	}
	recovery, err := runs.NewRecovery(persistence, alwaysResumable{})
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
	if err != nil || !found || stored.State != run.Failed ||
		stored.Error == nil || stored.Error.Kind != transcript.RunLostProblem {
		t.Fatalf("recovered Run = found:%t value:%+v err:%v", found, stored, err)
	}
	storedItem, found, err := transcriptStore.Item(ctx, item.ID)
	if err != nil || !found || storedItem.Status != transcript.ItemIncomplete {
		t.Fatalf("recovered Item = found:%t value:%+v err:%v", found, storedItem, err)
	}
	if _, err := checkpointStore.LoadCheckpoint(ctx, checkpoint.RootProcessID); !errors.Is(err, runs.ErrExecutorCheckpointNotFound) {
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
	if err := sessionStore.Restore(ctx, session.Session{
		ID: "session", CWD: "/workspace", StartedAt: createdAt, UpdatedAt: createdAt,
	}); err != nil {
		t.Fatalf("seed Session: %v", err)
	}
	interruptStore := persistence.NewInterruptStore(sqlite.NewInterruptStore(db))
	transcriptStore := sqlite.NewTranscriptStore(db)
	checkpointStore := persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db))
	if err := runStore.Admit(ctx, run.RunDraft{
		RunID: "run_partial", SessionID: "session", SegmentID: "segment", CreatedAt: createdAt,
		Capabilities: run.RunCapabilities{
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
	if err := runStore.Suspend(ctx, transcript.Run{
		ID: "run_partial", SessionID: "session", State: run.Waiting,
		Capabilities: run.RunCapabilities{
			InterruptKinds: []interrupt.Kind{interrupt.Question},
		},
		Interrupts: []transcript.Interrupt{pendingInterrupt}, CreatedAt: createdAt,
		UpdatedAt: createdAt.Add(time.Second), MessageMark: transcript.UnknownMessageMark,
	}); err != nil {
		t.Fatalf("Suspend: %v", err)
	}
	pending := runs.Pending{
		RootRunID: "run_partial", SessionID: "session", ExecutorID: "turn_partial",
		Capabilities: run.RunCapabilities{
			InterruptKinds: []interrupt.Kind{interrupt.Question},
		},
		Interrupts: []transcript.Interrupt{pendingInterrupt},
		Suspensions: []runs.SuspensionBinding{{
			InterruptItemID: pendingInterrupt.ItemID, ProcessID: "process_root", SuspensionID: "suspension_root",
		}},
		Continuations: []runs.Continuation{{
			RunID: "run_partial", ProcessID: "process_root", RunCreatedAt: createdAt,
		}},
		CreatedAt: createdAt.Add(time.Second),
	}
	if err := interruptStore.Open(ctx, pending); err != nil {
		t.Fatalf("Open Pending: %v", err)
	}
	checkpoint := runs.ExecutorCheckpoint{
		RootProcessID: "process_root", Payload: []byte(`{"opaque":true}`), BuildID: "build",
		Scope: runs.ExecutionScope{SessionID: "session"},
	}
	if err := checkpointStore.SaveCheckpoint(ctx, checkpoint); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}
	persistence, err := New(Config{
		Sessions: sessionStore, Runs: runStore, Interrupts: interruptStore, Transcript: transcriptStore,
		Messages: sqlite.NewMessageStore(db), GoalRuns: sqlite.NewGoalStore(db), ExecutorCheckpoints: checkpointStore,
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlite.RunInTx(ctx, db, fn)
		},
	})
	if err != nil {
		t.Fatalf("New persistence: %v", err)
	}
	recovery, err := runs.NewRecovery(persistence, alwaysResumable{})
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
	if err != nil || !found || stored.State != run.Waiting {
		t.Fatalf("Run after rejection = found:%t value:%+v err:%v", found, stored, err)
	}
	if _, err := checkpointStore.LoadCheckpoint(ctx, checkpoint.RootProcessID); err != nil {
		t.Fatalf("checkpoint after rejection: %v", err)
	}
}
