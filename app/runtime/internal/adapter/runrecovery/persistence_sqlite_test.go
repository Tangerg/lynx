package runrecovery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

type alwaysResumable struct{}

func (alwaysResumable) CanResumeCheckpoint(
	context.Context,
	execution.ExecutorCheckpointExpectation,
) (bool, error) {
	return true, nil
}

// TestRecoveryRepairsWholeDurableLifecycle proves
// terminal_run_explains_how_it_ended at the runs.recovery boundary.
func TestRecoveryRepairsWholeDurableLifecycle(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	createdAt := time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC)
	runStore := sqlite.NewRunStore(db)
	sessionStore := sqlite.NewSessionStore(db)
	if err := sessionStore.Restore(ctx, session.Session{
		ID: "session", Cwd: "/workspace", StartedAt: createdAt, UpdatedAt: createdAt,
	}); err != nil {
		t.Fatalf("seed Session: %v", err)
	}
	interruptStore := sqlite.NewInterruptStore(db)
	transcriptStore := sqlite.NewTranscriptStore(db)
	messageStore := sqlite.NewMessageStore(db)
	goalStore := sqlite.NewGoalStore(db)
	checkpointStore := sqlite.NewExecutorCheckpointStore(db)
	goalValue, err := goal.New("session", "finish recovery", modelref.Selection{}, goal.Budget{}, "lease_recovery", createdAt)
	if err != nil {
		t.Fatalf("New Goal: %v", err)
	}
	if _, applied, err := goalStore.Save(ctx, goalValue, goal.Version{}); err != nil || !applied {
		t.Fatalf("Save Goal: applied=%t err=%v", applied, err)
	}
	if err := runStore.Admit(ctx, execution.RunDraft{
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
	checkpoint := execution.ExecutorCheckpoint{
		RootProcessID: "orphan_checkpoint",
		Payload:       []byte(`{"opaque":true}`),
		BuildID:       "build",
		Scope:         execution.TurnScope{SessionID: "session"},
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
		GoalTurns:           goalStore,
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
	if err != nil || !found || stored.State != execution.Failed ||
		stored.Error == nil || stored.Error.Kind != transcript.RunLostProblem {
		t.Fatalf("recovered Run = found:%t value:%+v err:%v", found, stored, err)
	}
	storedItem, found, err := transcriptStore.Item(ctx, item.ID)
	if err != nil || !found || storedItem.Status != transcript.ItemIncomplete {
		t.Fatalf("recovered Item = found:%t value:%+v err:%v", found, storedItem, err)
	}
	if _, err := checkpointStore.LoadCheckpoint(ctx, checkpoint.RootProcessID); !errors.Is(err, execution.ErrExecutorCheckpointNotFound) {
		t.Fatalf("orphan checkpoint after recovery = %v", err)
	}
	storedGoal, found, err := goalStore.Get(ctx, goalValue.SessionID)
	if err != nil || !found || storedGoal.Used.Turns != 1 || storedGoal.Status != goal.StatusPaused || storedGoal.Reason.Cause != goal.ReasonRunNotCompleted {
		t.Fatalf("Goal after recovery = found:%t value:%+v err:%v", found, storedGoal, err)
	}
}

// TestRecoveryRejectsPartialParkWithoutMutatingIt proves
// parked_tree_has_exactly_one_open_interrupt_set at the runs.recovery boundary.
func TestRecoveryRejectsPartialParkWithoutMutatingIt(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	createdAt := time.Date(2026, 8, 1, 4, 0, 0, 0, time.UTC)
	runStore := sqlite.NewRunStore(db)
	sessionStore := sqlite.NewSessionStore(db)
	if err := sessionStore.Restore(ctx, session.Session{
		ID: "session", Cwd: "/workspace", StartedAt: createdAt, UpdatedAt: createdAt,
	}); err != nil {
		t.Fatalf("seed Session: %v", err)
	}
	interruptStore := sqlite.NewInterruptStore(db)
	transcriptStore := sqlite.NewTranscriptStore(db)
	checkpointStore := sqlite.NewExecutorCheckpointStore(db)
	if err := runStore.Admit(ctx, execution.RunDraft{
		RunID: "run_partial", SessionID: "session", SegmentID: "segment", CreatedAt: createdAt,
		ProtocolProfile: execution.RunProtocolProfile{
			InterruptKinds: []execution.InterruptKind{execution.QuestionInterrupt},
		},
	}); err != nil {
		t.Fatalf("Admit: %v", err)
	}
	question := &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?"}}}
	interrupt := transcript.Interrupt{
		ItemID: "item_missing", RunID: "run_partial", Kind: execution.QuestionInterrupt, Question: question,
	}
	if err := runStore.Suspend(ctx, transcript.Run{
		ID: "run_partial", SessionID: "session", State: execution.Interrupted,
		ProtocolProfile: execution.RunProtocolProfile{
			InterruptKinds: []execution.InterruptKind{execution.QuestionInterrupt},
		},
		Interrupts: []transcript.Interrupt{interrupt}, CreatedAt: createdAt,
		UpdatedAt: createdAt.Add(time.Second), MessageMark: transcript.UnknownMessageMark,
	}); err != nil {
		t.Fatalf("Suspend: %v", err)
	}
	pending := interrupts.Pending{
		RootRunID: "run_partial", SessionID: "session", TurnID: "turn_partial",
		ProtocolProfile: execution.RunProtocolProfile{
			InterruptKinds: []execution.InterruptKind{execution.QuestionInterrupt},
		},
		Interrupts: []transcript.Interrupt{interrupt},
		Suspensions: []interrupts.SuspensionBinding{{
			InterruptItemID: interrupt.ItemID, ProcessID: "process_root", SuspensionID: "suspension_root",
		}},
		Continuations: []interrupts.Continuation{{
			RunID: "run_partial", ProcessID: "process_root", RunCreatedAt: createdAt,
		}},
		CreatedAt: createdAt.Add(time.Second),
	}
	if err := interruptStore.Open(ctx, pending); err != nil {
		t.Fatalf("Open Pending: %v", err)
	}
	checkpoint := execution.ExecutorCheckpoint{
		RootProcessID: "process_root", Payload: []byte(`{"opaque":true}`), BuildID: "build",
		Scope: execution.TurnScope{SessionID: "session"},
	}
	if err := checkpointStore.SaveCheckpoint(ctx, checkpoint); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}
	persistence, err := New(Config{
		Sessions: sessionStore, Runs: runStore, Interrupts: interruptStore, Transcript: transcriptStore,
		Messages: sqlite.NewMessageStore(db), GoalTurns: sqlite.NewGoalStore(db), ExecutorCheckpoints: checkpointStore,
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
	if err != nil || !found || stored.State != execution.Interrupted {
		t.Fatalf("Run after rejection = found:%t value:%+v err:%v", found, stored, err)
	}
	if _, err := checkpointStore.LoadCheckpoint(ctx, checkpoint.RootProcessID); err != nil {
		t.Fatalf("checkpoint after rejection: %v", err)
	}
}
