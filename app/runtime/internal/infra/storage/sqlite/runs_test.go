package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
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

func newRunRecoveryStores(t *testing.T) (*sqlite.RunStateStore, *sqlite.InterruptStore, *sqlite.TranscriptStore, *sqlite.ProcessStore) {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewRunStateStore(db), sqlite.NewInterruptStore(db), sqlite.NewTranscriptStore(db), sqlite.NewProcessStore(db)
}

func acceptProcessSnapshot(context.Context, string) (bool, error) { return true, nil }

func putActiveTranscript(t *testing.T, store *sqlite.TranscriptStore, runID, sessionID string, state execution.RunState) {
	t.Helper()
	if err := store.PutRun(t.Context(), transcript.Run{
		SessionID: sessionID, ID: runID, State: state,
		CreatedAt: time.Unix(1, 0), UpdatedAt: time.Unix(1, 0), MessageMark: -1,
	}); err != nil {
		t.Fatalf("put active transcript: %v", err)
	}
}

func putParkedState(t *testing.T, transcripts *sqlite.TranscriptStore, ints *sqlite.InterruptStore, processes *sqlite.ProcessStore, runID, sessionID string) {
	t.Helper()
	createdAt := time.Unix(1, 0).UTC()
	parkedAt := time.Unix(2, 0).UTC()
	question := &transcript.Question{Prompt: "Continue?"}
	open := []transcript.Interrupt{{
		ItemID: "item_" + runID, Kind: transcript.QuestionInterrupt, Question: question,
	}}
	if err := transcripts.PutRun(t.Context(), transcript.Run{
		SessionID: sessionID, ID: runID, State: execution.Interrupted,
		Interrupts: open, CreatedAt: createdAt, UpdatedAt: parkedAt, MessageMark: -1,
	}); err != nil {
		t.Fatalf("put parked transcript run: %v", err)
	}
	if err := transcripts.AppendItem(t.Context(), transcript.Item{
		SessionID: sessionID, ID: "item_" + runID, RunID: runID,
		Status: transcript.ItemRunning, Kind: transcript.QuestionItem,
		Question: question, CreatedAt: parkedAt,
	}); err != nil {
		t.Fatalf("put parked transcript item: %v", err)
	}
	if err := ints.Put(t.Context(), interrupts.Pending{
		RunID: runID, SessionID: sessionID, TurnID: "turn_" + runID,
		ProcessID: "proc_" + runID, Interrupts: open,
		RunCreatedAt: createdAt, CreatedAt: parkedAt,
	}); err != nil {
		t.Fatalf("put parked interrupt: %v", err)
	}
	snapshot := validStoredSnapshot("proc_"+runID, core.StatusWaiting)
	snapshot.StartedAt = createdAt
	snapshot.CapturedAt = parkedAt
	if _, err := processes.Save(t.Context(), snapshot, 0); err != nil {
		t.Fatalf("put parked process snapshot: %v", err)
	}
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
// the row, while a cancel of the same parked run succeeds.
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
	store, ints, transcripts, processes := newRunRecoveryStores(t)

	// Orphaned interrupted row: interrupted state but no interrupt record.
	if err := store.Admit(ctx, runDraft("run_orphan", "ses_orphan")); err != nil {
		t.Fatalf("admit orphan: %v", err)
	}
	if err := store.Suspend(ctx, "ses_orphan", "run_orphan"); err != nil {
		t.Fatalf("suspend orphan: %v", err)
	}
	putActiveTranscript(t, transcripts, "run_orphan", "ses_orphan", execution.Interrupted)
	// Genuinely parked: interrupted state WITH an open interrupt record.
	if err := store.Admit(ctx, runDraft("run_park", "ses_park")); err != nil {
		t.Fatalf("admit park: %v", err)
	}
	if err := store.Suspend(ctx, "ses_park", "run_park"); err != nil {
		t.Fatalf("suspend park: %v", err)
	}
	putParkedState(t, transcripts, ints, processes, "run_park", "ses_park")

	swept, err := store.ReconcileOrphans(ctx, acceptProcessSnapshot)
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
// leaves an interrupted run whose matching interrupt makes it resumable.
func TestReconcileOrphansSweepsCrashedButPreservesParked(t *testing.T) {
	ctx := context.Background()
	store, ints, transcripts, processes := newRunRecoveryStores(t)

	if err := store.Admit(ctx, runDraft("run_crash", "ses_crash")); err != nil {
		t.Fatalf("admit crash: %v", err)
	}
	putActiveTranscript(t, transcripts, "run_crash", "ses_crash", execution.Running)
	if err := store.Admit(ctx, runDraft("run_park", "ses_park")); err != nil {
		t.Fatalf("admit park: %v", err)
	}
	if err := store.Suspend(ctx, "ses_park", "run_park"); err != nil {
		t.Fatalf("suspend park: %v", err)
	}
	putParkedState(t, transcripts, ints, processes, "run_park", "ses_park")

	swept, err := store.ReconcileOrphans(ctx, acceptProcessSnapshot)
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

func TestReconcileOrphansTerminalizesParkWhoseProcessSnapshotIsMissing(t *testing.T) {
	store, ints, transcripts, processes := newRunRecoveryStores(t)
	ctx := t.Context()
	if err := store.Admit(ctx, runDraft("run_park", "ses_park")); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := store.Suspend(ctx, "ses_park", "run_park"); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	putParkedState(t, transcripts, ints, processes, "run_park", "ses_park")
	if err := processes.Delete(ctx, "proc_run_park"); err != nil {
		t.Fatalf("delete process snapshot: %v", err)
	}

	recovered, err := store.ReconcileOrphans(ctx, func(context.Context, string) (bool, error) {
		return false, nil
	})
	if err != nil || recovered != 1 {
		t.Fatalf("reconcile = (%d, %v), want one recovered lost park", recovered, err)
	}
	if pending, err := ints.List(ctx, "ses_park"); err != nil || len(pending) != 0 {
		t.Fatalf("pending after recovery = (%+v, %v), want none", pending, err)
	}
	_, runs, err := transcripts.List(ctx, "ses_park")
	if err != nil || len(runs) != 1 || runs[0].State != execution.Failed || runs[0].Result == nil || runs[0].Result.Error == nil || runs[0].Result.Error.Kind != transcript.RunLostProblem {
		t.Fatalf("transcript after recovery = (%+v, %v), want failed run_lost", runs, err)
	}
	if err := store.Admit(ctx, runDraft("run_next", "ses_park")); err != nil {
		t.Fatalf("admit after lost park recovery: %v", err)
	}
}

func TestReconcileOrphansRepairsWholeDurableLifecycle(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	runStore := sqlite.NewRunStateStore(db)
	transcripts := sqlite.NewTranscriptStore(db)

	if err := runStore.Admit(ctx, runDraft("run_lost", "ses_lost")); err != nil {
		t.Fatalf("admit lost run: %v", err)
	}
	if err := transcripts.PutRun(ctx, transcript.Run{
		SessionID: "ses_lost", ID: "run_lost", State: execution.Running,
		CreatedAt: time.Unix(1, 0), UpdatedAt: time.Unix(1, 0), MessageMark: -1,
	}); err != nil {
		t.Fatalf("put running transcript: %v", err)
	}
	if err := transcripts.AppendItem(ctx, transcript.Item{
		SessionID: "ses_lost", ID: "item_tool", RunID: "run_lost",
		Status: transcript.ItemRunning, Kind: transcript.ToolCall, CreatedAt: time.Unix(2, 0),
		Tool: &transcript.ToolInvocation{Name: "shell"},
	}); err != nil {
		t.Fatalf("put running item: %v", err)
	}

	swept, err := runStore.ReconcileOrphans(ctx, acceptProcessSnapshot)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if swept != 1 {
		t.Fatalf("swept = %d, want 1", swept)
	}
	items, runs, err := transcripts.List(ctx, "ses_lost")
	if err != nil {
		t.Fatalf("list transcript: %v", err)
	}
	if len(runs) != 1 || runs[0].State != execution.Failed || runs[0].Outcome == nil || *runs[0].Outcome != execution.OutcomeError {
		t.Fatalf("recovered run = %+v, want failed/error", runs)
	}
	if runs[0].Result == nil || runs[0].Result.Error == nil || runs[0].Result.Error.Kind != transcript.RunLostProblem {
		t.Fatalf("recovered run result = %+v, want run-lost problem", runs[0].Result)
	}
	if runs[0].FinishedAt.IsZero() || runs[0].MessageMark != 0 {
		t.Fatalf("recovered terminal boundary = finished:%v mark:%d", runs[0].FinishedAt, runs[0].MessageMark)
	}
	if len(items) != 1 || items[0].Status != transcript.ItemIncomplete || items[0].Error == nil {
		t.Fatalf("recovered items = %+v, want incomplete failed tool", items)
	}
	if err := runStore.Admit(ctx, runDraft("run_next", "ses_lost")); err != nil {
		t.Fatalf("re-admit after full recovery: %v", err)
	}
}

func TestReconcileOrphansDoesNotLetStaleInterruptProtectRunningRun(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewRunStateStore(db)
	interruptStore := sqlite.NewInterruptStore(db)
	processStore := sqlite.NewProcessStore(db)
	transcripts := sqlite.NewTranscriptStore(db)
	ctx := t.Context()
	if err := store.Admit(ctx, runDraft("run_lost", "ses_1")); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := transcripts.PutRun(ctx, transcript.Run{
		SessionID: "ses_1", ID: "run_lost", State: execution.Running,
		CreatedAt: time.Unix(1, 0), UpdatedAt: time.Unix(1, 0), MessageMark: -1,
	}); err != nil {
		t.Fatalf("put transcript: %v", err)
	}
	if _, err := processStore.Save(ctx, validStoredSnapshot("proc_stale", core.StatusWaiting), 0); err != nil {
		t.Fatalf("put stale process snapshot: %v", err)
	}
	if err := interruptStore.Put(ctx, interrupts.Pending{RunID: "run_stale", SessionID: "ses_1", ProcessID: "proc_stale", CreatedAt: time.Unix(0, 0)}); err != nil {
		t.Fatalf("put stale interrupt: %v", err)
	}
	if swept, err := store.ReconcileOrphans(ctx, acceptProcessSnapshot); err != nil || swept != 1 {
		t.Fatalf("reconcile = (%d, %v), want (1, nil)", swept, err)
	}
	if pending, err := interruptStore.List(ctx, "ses_1"); err != nil || len(pending) != 0 {
		t.Fatalf("stale interrupts after reconcile = (%+v, %v), want none", pending, err)
	}
	if _, err := processStore.Load(ctx, "proc_stale"); !errors.Is(err, core.ErrSnapshotNotFound) {
		t.Fatalf("stale process snapshot after reconcile = %v, want not found", err)
	}
}

func TestReconcileOrphansRejectsPartialParkWithoutMutatingIt(t *testing.T) {
	store, ints, transcripts, _ := newRunRecoveryStores(t)
	ctx := t.Context()
	if err := store.Admit(ctx, runDraft("run_park", "ses_park")); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := store.Suspend(ctx, "ses_park", "run_park"); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	createdAt := time.Unix(1, 0).UTC()
	parkedAt := time.Unix(2, 0).UTC()
	question := &transcript.Question{Prompt: "Continue?"}
	open := []transcript.Interrupt{{ItemID: "item_missing", Kind: transcript.QuestionInterrupt, Question: question}}
	if err := transcripts.PutRun(ctx, transcript.Run{
		SessionID: "ses_park", ID: "run_park", State: execution.Interrupted,
		Interrupts: open, CreatedAt: createdAt, UpdatedAt: parkedAt, MessageMark: -1,
	}); err != nil {
		t.Fatalf("put transcript: %v", err)
	}
	if err := ints.Put(ctx, interrupts.Pending{
		RunID: "run_park", SessionID: "ses_park", TurnID: "turn_park", ProcessID: "proc_park",
		Interrupts: open, RunCreatedAt: createdAt, CreatedAt: parkedAt,
	}); err != nil {
		t.Fatalf("put interrupt: %v", err)
	}

	if _, err := store.ReconcileOrphans(ctx, acceptProcessSnapshot); err == nil {
		t.Fatal("reconcile accepted a parked run whose interrupt item is missing")
	}
	if err := store.Admit(ctx, runDraft("run_next", "ses_park")); !errors.Is(err, execution.ErrSessionBusy) {
		t.Fatalf("admit after rejected recovery = %v, want original park to remain busy", err)
	}
	if _, found, err := ints.Get(ctx, "run_park"); err != nil || !found {
		t.Fatalf("interrupt after rejected recovery = found:%v err:%v, want preserved transaction", found, err)
	}
}

func TestReconcileOrphansRejectsUnmappedRunningItem(t *testing.T) {
	store, ints, transcripts, processes := newRunRecoveryStores(t)
	ctx := t.Context()
	if err := store.Admit(ctx, runDraft("run_park", "ses_park")); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := store.Suspend(ctx, "ses_park", "run_park"); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	putParkedState(t, transcripts, ints, processes, "run_park", "ses_park")
	if err := transcripts.AppendItem(ctx, transcript.Item{
		SessionID: "ses_park", RunID: "run_park", ID: "item_unmapped",
		Status: transcript.ItemRunning, Kind: transcript.QuestionItem,
		Question: &transcript.Question{Prompt: "orphan"}, CreatedAt: time.Unix(3, 0),
	}); err != nil {
		t.Fatalf("append unmapped item: %v", err)
	}

	if _, err := store.ReconcileOrphans(ctx, acceptProcessSnapshot); err == nil {
		t.Fatal("reconcile accepted a running item with no matching interrupt")
	}
	if _, found, err := ints.Get(ctx, "run_park"); err != nil || !found {
		t.Fatalf("interrupt after rejected recovery = found:%v err:%v, want preserved", found, err)
	}
}

func TestReconcileOrphansRejectsParkModelMismatch(t *testing.T) {
	store, ints, transcripts, processes := newRunRecoveryStores(t)
	ctx := t.Context()
	if err := store.Admit(ctx, execution.RunDraft{
		RunID: "run_park", SessionID: "ses_park", Provider: "openai", Model: "gpt-test", CreatedAt: time.Unix(0, 0),
	}); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := store.Suspend(ctx, "ses_park", "run_park"); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	putParkedState(t, transcripts, ints, processes, "run_park", "ses_park")

	if _, err := store.ReconcileOrphans(ctx, acceptProcessSnapshot); err == nil {
		t.Fatal("reconcile accepted a park whose model differs from admission")
	}
}

func TestReconcileOrphansRejectsDrainedInterruptOverlap(t *testing.T) {
	store, ints, transcripts, processes := newRunRecoveryStores(t)
	ctx := t.Context()
	if err := store.Admit(ctx, runDraft("run_park", "ses_park")); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := store.Suspend(ctx, "ses_park", "run_park"); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	putParkedState(t, transcripts, ints, processes, "run_park", "ses_park")
	pending, found, err := ints.Get(ctx, "run_park")
	if err != nil || !found {
		t.Fatalf("get pending: found=%v err=%v", found, err)
	}
	pending.DrainedTools = []interrupts.DrainedTool{{ItemID: "item_run_park", Name: "ask_user"}}
	if err := ints.Put(ctx, pending); err != nil {
		t.Fatalf("replace pending: %v", err)
	}

	if _, err := store.ReconcileOrphans(ctx, acceptProcessSnapshot); err == nil {
		t.Fatal("reconcile accepted one item as both interrupt and drained tool")
	}
}

func TestReconcileOrphansTerminalizesExecutorIncompatibleSnapshot(t *testing.T) {
	store, ints, transcripts, processes := newRunRecoveryStores(t)
	ctx := t.Context()
	if err := store.Admit(ctx, runDraft("run_park", "ses_park")); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := store.Suspend(ctx, "ses_park", "run_park"); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	putParkedState(t, transcripts, ints, processes, "run_park", "ses_park")

	recovered, err := store.ReconcileOrphans(ctx, func(context.Context, string) (bool, error) {
		return false, nil
	})
	if err != nil || recovered != 1 {
		t.Fatalf("reconcile = (%d, %v), want one recovered incompatible park", recovered, err)
	}
	if _, found, err := ints.Get(ctx, "run_park"); err != nil || found {
		t.Fatalf("interrupt after incompatible snapshot = found:%v err:%v, want removed", found, err)
	}
	if _, err := processes.Load(ctx, "proc_run_park"); !errors.Is(err, core.ErrSnapshotNotFound) {
		t.Fatalf("snapshot after incompatible recovery = %v, want not found", err)
	}
	_, runs, err := transcripts.List(ctx, "ses_park")
	if err != nil || len(runs) != 1 || runs[0].Result == nil || runs[0].Result.Error == nil || runs[0].Result.Error.Kind != transcript.RunLostProblem {
		t.Fatalf("transcript after incompatible recovery = (%+v, %v), want run_lost", runs, err)
	}
}

func TestReconcileOrphansRejectsSnapshotValidatorFailureWithoutMutation(t *testing.T) {
	store, ints, transcripts, processes := newRunRecoveryStores(t)
	ctx := t.Context()
	if err := store.Admit(ctx, runDraft("run_park", "ses_park")); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := store.Suspend(ctx, "ses_park", "run_park"); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	putParkedState(t, transcripts, ints, processes, "run_park", "ses_park")
	want := errors.New("missing executor tail")
	if _, err := store.ReconcileOrphans(ctx, func(context.Context, string) (bool, error) { return false, want }); !errors.Is(err, want) {
		t.Fatalf("reconcile error = %v, want executor snapshot error", err)
	}
	if _, found, err := ints.Get(ctx, "run_park"); err != nil || !found {
		t.Fatalf("interrupt after rejected snapshot = found:%v err:%v, want preserved", found, err)
	}
	if err := store.Admit(ctx, runDraft("run_next", "ses_park")); !errors.Is(err, execution.ErrSessionBusy) {
		t.Fatalf("admit after rejected snapshot = %v, want original park busy", err)
	}
}

func TestTerminalizeRejectsUnknownOutcome(t *testing.T) {
	store, _ := newRunStores(t)
	ctx := t.Context()
	if err := store.Admit(ctx, runDraft("run_1", "ses_1")); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := store.Terminalize(ctx, "ses_1", "run_1", execution.Outcome(255)); err == nil {
		t.Fatal("terminalize accepted an unknown outcome")
	}
}
