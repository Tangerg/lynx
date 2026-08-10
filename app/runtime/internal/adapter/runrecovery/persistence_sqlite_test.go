package runrecovery

import (
	"context"
	"errors"
	"reflect"
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
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/itemfixture"
	runfixture "github.com/Tangerg/lynx/app/runtime/internal/testsupport/runfixture"
)

type alwaysResumable struct{}

func (alwaysResumable) CanResumeCheckpoint(
	context.Context,
	runs.ExecutorCheckpointExpectation,
) (bool, error) {
	return true, nil
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
	if err := sessionStore.Restore(ctx, session.Session{
		ID: "session_claim", CWD: "/workspace", StartedAt: createdAt, UpdatedAt: createdAt,
	}); err != nil {
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

	checkpointStore := persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db))
	store, err := New(Config{
		Sessions: sessionStore, Runs: runStore, Interrupts: interruptStore,
		Transcript: sqlite.NewTranscriptStore(db), Messages: sqlite.NewMessageStore(db),
		GoalRuns: sqlite.NewGoalStore(db), ExecutorCheckpoints: checkpointStore,
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlite.RunInTx(ctx, db, fn)
		},
	})
	if err != nil {
		t.Fatalf("New persistence: %v", err)
	}
	checkpointProbes := 0
	recovery, err := runs.NewRecovery(store, checkpointResumabilityFunc(func(
		context.Context,
		runs.ExecutorCheckpointExpectation,
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
		!failed || failure.Kind != run.FailureLost {
		t.Fatalf("recovered Run = found:%t value:%+v err:%v", found, stored, err)
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

type checkpointResumabilityFunc func(context.Context, runs.ExecutorCheckpointExpectation) (bool, error)

func (probe checkpointResumabilityFunc) CanResumeCheckpoint(
	ctx context.Context,
	expected runs.ExecutorCheckpointExpectation,
) (bool, error) {
	return probe(ctx, expected)
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
	if err := runStore.Admit(ctx, run.Draft{
		RunID: "run_lost", SessionID: "session", SegmentID: "segment", GoalLeaseID: goalValue.LeaseID, CreatedAt: createdAt,
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
	checkpoint := runs.ExecutorCheckpoint{
		RootMemberID: "orphan_checkpoint",
		Payload:      []byte(`{"opaque":true}`),
		BuildID:      "build",
		Scope:        runs.ExecutionScope{SessionID: "session"},
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
	failure, failed := stored.Failure()
	if err != nil || !found || stored.State() != run.Failed ||
		!failed || failure.Kind != run.FailureLost {
		t.Fatalf("recovered Run = found:%t value:%+v err:%v", found, stored, err)
	}
	storedItem, found, err := transcriptStore.Item(ctx, item.ID())
	if err != nil || !found || storedItem.Status() != transcript.ItemCompleted ||
		!reflect.DeepEqual(storedItem.Snapshot(), item.Snapshot()) {
		t.Fatalf("recovered Item = found:%t value:%+v err:%v", found, storedItem, err)
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
	if err := sessionStore.Restore(ctx, session.Session{
		ID: "session", CWD: "/workspace", StartedAt: createdAt, UpdatedAt: createdAt,
	}); err != nil {
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
	if err != nil || !found || stored.State() != run.Waiting {
		t.Fatalf("Run after rejection = found:%t value:%+v err:%v", found, stored, err)
	}
	if _, err := checkpointStore.LoadCheckpoint(ctx, checkpoint.RootMemberID); err != nil {
		t.Fatalf("checkpoint after rejection: %v", err)
	}
}
