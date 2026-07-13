package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

func newRunStores(t *testing.T) (*sqlite.RunStateStore, *sqlite.InterruptStore) {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewRunStateStore(db), sqlite.NewInterruptStore(db)
}

func runDraft(runID, sessionID string) execution.RunDraft {
	return execution.RunDraft{RunID: runID, SessionID: sessionID, CreatedAt: time.Unix(0, 0)}
}

// TestParkCommitsInterruptAndSuspendAtomically proves the §8.3 pairing the
// run-event committer relies on: opening the interrupt record and suspending the
// run's admission row commit — or roll back — as ONE transaction (both writes
// join the same conn(ctx)), so a crash can never leave a parked run with an
// interrupt but a still-running admission row, or vice versa.
func TestParkCommitsInterruptAndSuspendAtomically(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	runStore, ints := sqlite.NewRunStateStore(db), sqlite.NewInterruptStore(db)
	ctx := context.Background()

	if err := runStore.Admit(ctx, runDraft("run_1", "ses_A")); err != nil {
		t.Fatalf("admit: %v", err)
	}

	// park writes through the transaction's context so both statements join the
	// SAME connection (conn(ctx)); using the outer ctx would open a second
	// connection under MaxOpenConns(1) and deadlock.
	park := func(ctx context.Context) error {
		if err := ints.Put(ctx, interrupts.Pending{RunID: "run_1", SessionID: "ses_A", CreatedAt: time.Unix(0, 0)}); err != nil {
			return err
		}
		return runStore.Suspend(ctx, "ses_A", "run_1")
	}

	// A park commit that fails after both writes leaves NEITHER: no interrupt, and
	// the row stays running (a second admit is still rejected as busy).
	boom := errors.New("boom")
	if err := sqlite.RunInTx(ctx, db, func(ctx context.Context) error {
		if err := park(ctx); err != nil {
			return err
		}
		return boom
	}); !errors.Is(err, boom) {
		t.Fatalf("RunInTx err = %v, want boom", err)
	}
	if open, _ := ints.List(ctx, "ses_A"); len(open) != 0 {
		t.Fatalf("interrupt survived a rolled-back park: %+v", open)
	}
	// Still running (not interrupted): a rolled-back Suspend left the state intact.
	if err := runStore.Resume(ctx, execution.ResumeDraft{RunID: "run_1", SessionID: "ses_A"}); err == nil {
		t.Fatal("resume after rolled-back park must reject the still-running row")
	}
	if err := runStore.Admit(ctx, runDraft("run_x", "ses_A")); !errors.Is(err, execution.ErrSessionBusy) {
		t.Fatalf("admit after rolled-back park = %v, want ErrSessionBusy (row never freed)", err)
	}

	// A successful park commit persists BOTH the interrupt and the suspended state.
	if err := sqlite.RunInTx(ctx, db, park); err != nil {
		t.Fatalf("park commit: %v", err)
	}
	if open, _ := ints.List(ctx, "ses_A"); len(open) != 1 {
		t.Fatalf("open interrupts = %d, want 1 after committed park", len(open))
	}
	if err := runStore.Admit(ctx, runDraft("run_y", "ses_A")); !errors.Is(err, execution.ErrSessionBusy) {
		t.Fatalf("admit while parked = %v, want ErrSessionBusy (row non-terminal)", err)
	}
}

// TestRunAdmitEnforcesOneActivePerSession proves the durable §8.2 guarantee: the
// partial unique index rejects a second non-terminal run for the same session,
// a different session is independent, and terminalizing frees the slot.
func TestRunAdmitEnforcesOneActivePerSession(t *testing.T) {
	ctx := context.Background()
	store, _ := newRunStores(t)

	if err := store.Admit(ctx, runDraft("run_1", "ses_A")); err != nil {
		t.Fatalf("first admit: %v", err)
	}
	if err := store.Admit(ctx, runDraft("run_2", "ses_A")); !errors.Is(err, execution.ErrSessionBusy) {
		t.Fatalf("second admit err = %v, want ErrSessionBusy", err)
	}
	if err := store.Admit(ctx, runDraft("run_3", "ses_B")); err != nil {
		t.Fatalf("other-session admit: %v", err)
	}
	if err := store.Terminalize(ctx, "ses_A", "run_1", execution.OutcomeCompleted); err != nil {
		t.Fatalf("terminalize: %v", err)
	}
	if err := store.Admit(ctx, runDraft("run_4", "ses_A")); err != nil {
		t.Fatalf("re-admit after terminal: %v", err)
	}
}

// TestTerminalizeRequiresExactLiveRun pins strict lifecycle ownership: an
// unknown, mismatched, or already-terminal run is an error, never a session-
// scoped no-op that can hide a duplicated terminal decision.
func TestTerminalizeRequiresExactLiveRun(t *testing.T) {
	ctx := context.Background()
	store, _ := newRunStores(t)

	if err := store.Terminalize(ctx, "ses_unknown", "run_unknown", execution.OutcomeCompleted); err == nil {
		t.Fatal("terminalize unknown run must fail")
	}
	if err := store.Admit(ctx, runDraft("run_1", "ses_A")); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := store.Terminalize(ctx, "ses_A", "run_other", execution.OutcomeCompleted); err == nil {
		t.Fatal("terminalize mismatched run must fail")
	}
	if err := store.Terminalize(ctx, "ses_A", "run_1", execution.OutcomeCompleted); err != nil {
		t.Fatalf("terminalize: %v", err)
	}
	if err := store.Terminalize(ctx, "ses_A", "run_1", execution.OutcomeCompleted); err == nil {
		t.Fatal("repeated terminalize must fail")
	}
}

// TestTerminalizeParkedRunRejectsNonCancel proves the store defers to the
// [execution.RunState] machine (§8.2): a parked (interrupted) run may terminalize
// only via cancellation — any other terminal must resume first — so a non-cancel
// terminalize of a parked run surfaces an error instead of silently overwriting
// the row, while a cancel of the same parked run succeeds (the parked-cancel path
// ApplyCancel relies on).
func TestTerminalizeParkedRunRejectsNonCancel(t *testing.T) {
	ctx := context.Background()
	store, _ := newRunStores(t)

	if err := store.Admit(ctx, runDraft("run_1", "ses_A")); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := store.Suspend(ctx, "ses_A", "run_1"); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	// A parked run cannot complete/error/cap out without resuming — the illegal
	// transition is surfaced, not silently applied.
	if err := store.Terminalize(ctx, "ses_A", "run_1", execution.OutcomeCompleted); err == nil {
		t.Fatal("terminalize(completed) of a parked run must be rejected as illegal")
	}
	// The row is untouched — still non-terminal, still busy.
	if err := store.Admit(ctx, runDraft("run_2", "ses_A")); !errors.Is(err, execution.ErrSessionBusy) {
		t.Fatalf("admit after rejected terminalize = %v, want ErrSessionBusy (row untouched)", err)
	}
	// Cancellation of the same parked run is legal (Interrupted → Canceled).
	if err := store.Terminalize(ctx, "ses_A", "run_1", execution.OutcomeCanceled); err != nil {
		t.Fatalf("terminalize(canceled) of a parked run: %v", err)
	}
	if err := store.Admit(ctx, runDraft("run_3", "ses_A")); err != nil {
		t.Fatalf("re-admit after parked cancel: %v", err)
	}
}

// TestSuspendResumeReusesOneSlot: a parked run's Suspend keeps the session's row
// non-terminal (a second Admit is still rejected), and a continuation's Resume
// keeps reusing that one row rather than admitting a second — so the durable
// slot survives the full park→resume→park→terminal cycle. Terminalize after
// resume frees it.
func TestSuspendResumeReusesOneSlot(t *testing.T) {
	ctx := context.Background()
	store, _ := newRunStores(t)

	if err := store.Admit(ctx, runDraft("run_1", "ses_A")); err != nil {
		t.Fatalf("admit: %v", err)
	}
	// Park: the row goes interrupted but stays non-terminal — still busy.
	if err := store.Suspend(ctx, "ses_A", "run_1"); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if err := store.Admit(ctx, runDraft("run_2", "ses_A")); !errors.Is(err, execution.ErrSessionBusy) {
		t.Fatalf("admit while suspended = %v, want ErrSessionBusy (row still non-terminal)", err)
	}
	// Resume: back to running, no second row admitted.
	if err := store.Resume(ctx, execution.ResumeDraft{RunID: "run_1", SessionID: "ses_A"}); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if err := store.Admit(ctx, runDraft("run_3", "ses_A")); !errors.Is(err, execution.ErrSessionBusy) {
		t.Fatalf("admit while resumed = %v, want ErrSessionBusy", err)
	}
	// Terminal frees the one reused slot.
	if err := store.Terminalize(ctx, "ses_A", "run_1", execution.OutcomeCompleted); err != nil {
		t.Fatalf("terminalize: %v", err)
	}
	if err := store.Admit(ctx, runDraft("run_4", "ses_A")); err != nil {
		t.Fatalf("re-admit after terminal: %v", err)
	}
}

// TestDeleteForSessionFreesSlot: dropping a session's rows wholesale (the
// delete/restore cascade) removes even a non-terminal row, freeing the session.
func TestDeleteForSessionFreesSlot(t *testing.T) {
	ctx := context.Background()
	store, _ := newRunStores(t)

	if err := store.Admit(ctx, runDraft("run_1", "ses_A")); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := store.DeleteForSession(ctx, "ses_A"); err != nil {
		t.Fatalf("delete for session: %v", err)
	}
	// The non-terminal row is gone, so a fresh admit succeeds.
	if err := store.Admit(ctx, runDraft("run_2", "ses_A")); err != nil {
		t.Fatalf("re-admit after delete: %v", err)
	}
}

// TestReconcileOrphansSweepsInterruptedWithoutRecord: the boot sweep reclaims a
// non-terminal run — running OR interrupted — whose session has no open
// interrupt (a continuation whose consumed-interrupt Resume was missed leaves an
// interrupted-but-orphaned row), while a genuinely parked interrupted run (its
// interrupt still recorded) is preserved.
func TestReconcileOrphansSweepsInterruptedWithoutRecord(t *testing.T) {
	ctx := context.Background()
	store, ints := newRunStores(t)

	// Orphaned interrupted row: interrupted state but no interrupt record.
	if err := store.Admit(ctx, runDraft("run_orphan", "ses_orphan")); err != nil {
		t.Fatalf("admit orphan: %v", err)
	}
	if err := store.Suspend(ctx, "ses_orphan", "run_orphan"); err != nil {
		t.Fatalf("suspend orphan: %v", err)
	}
	// Genuinely parked: interrupted state WITH an open interrupt record.
	if err := store.Admit(ctx, runDraft("run_park", "ses_park")); err != nil {
		t.Fatalf("admit park: %v", err)
	}
	if err := store.Suspend(ctx, "ses_park", "run_park"); err != nil {
		t.Fatalf("suspend park: %v", err)
	}
	if err := ints.Put(ctx, interrupts.Pending{
		RunID: "run_park", SessionID: "ses_park", CreatedAt: time.Unix(0, 0),
	}); err != nil {
		t.Fatalf("put interrupt: %v", err)
	}

	swept, err := store.ReconcileOrphans(ctx)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if swept != 1 {
		t.Fatalf("swept = %d, want 1 (only the interrupted orphan without a record)", swept)
	}
	if err := store.Admit(ctx, runDraft("run_orphan2", "ses_orphan")); err != nil {
		t.Fatalf("re-admit swept orphan session: %v", err)
	}
	if err := store.Admit(ctx, runDraft("run_park2", "ses_park")); !errors.Is(err, execution.ErrSessionBusy) {
		t.Fatalf("parked-session admit = %v, want ErrSessionBusy (preserved)", err)
	}
}

// TestReconcileOrphansSweepsCrashedButPreservesParked: a boot sweep terminalizes
// a running run whose process is gone with no open interrupt (a crash), but
// leaves a running run whose session has a parked interrupt (resumable) — so a
// crash frees its session while parked runs stay resumable across restart.
func TestReconcileOrphansSweepsCrashedButPreservesParked(t *testing.T) {
	ctx := context.Background()
	store, ints := newRunStores(t)

	if err := store.Admit(ctx, runDraft("run_crash", "ses_crash")); err != nil {
		t.Fatalf("admit crash: %v", err)
	}
	if err := store.Admit(ctx, runDraft("run_park", "ses_park")); err != nil {
		t.Fatalf("admit park: %v", err)
	}
	if err := ints.Put(ctx, interrupts.Pending{
		RunID: "run_park", SessionID: "ses_park", CreatedAt: time.Unix(0, 0),
	}); err != nil {
		t.Fatalf("put interrupt: %v", err)
	}

	swept, err := store.ReconcileOrphans(ctx)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if swept != 1 {
		t.Fatalf("swept = %d, want 1 (only the crashed orphan)", swept)
	}
	if err := store.Admit(ctx, runDraft("run_crash2", "ses_crash")); err != nil {
		t.Fatalf("re-admit swept session: %v", err)
	}
	if err := store.Admit(ctx, runDraft("run_park2", "ses_park")); !errors.Is(err, execution.ErrSessionBusy) {
		t.Fatalf("parked-session admit err = %v, want ErrSessionBusy (preserved)", err)
	}
}
