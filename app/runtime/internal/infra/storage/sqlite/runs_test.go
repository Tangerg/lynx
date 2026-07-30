package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

func newRunStores(t *testing.T) (*sqlite.RunStore, *sqlite.InterruptStore) {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewRunStore(db), sqlite.NewInterruptStore(db)
}

func newRunRecoveryStores(t *testing.T) (*sqlite.RunStore, *sqlite.InterruptStore, *sqlite.TranscriptStore, *sqlite.ProcessStore) {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewRunStore(db), sqlite.NewInterruptStore(db), sqlite.NewTranscriptStore(db), sqlite.NewProcessStore(db)
}

func acceptProcessSnapshot(context.Context, string) (bool, error) { return true, nil }

// runCreatedAt is when every fixture Run is admitted. The interrupt record
// carries the same instant, and a park whose two records disagree about when its
// Run started is rejected as an incomplete boundary.
var runCreatedAt = time.Unix(1, 0).UTC()

// putParkedState completes a park around an already-suspended Run: the open
// interrupt, the running item it refers to, and the resumable process snapshot.
// The Run's own row needs nothing added — Suspend is what parked it.
func putParkedState(t *testing.T, transcripts *sqlite.TranscriptStore, ints *sqlite.InterruptStore, processes *sqlite.ProcessStore, runID, sessionID string) {
	t.Helper()
	createdAt := runCreatedAt
	parkedAt := time.Unix(2, 0).UTC()
	question := &transcript.Question{Prompt: "Continue?"}
	open := []transcript.Interrupt{{
		ItemID: "item_" + runID, RunID: runID, Kind: execution.QuestionInterrupt, Question: question,
	}}
	if err := transcripts.AppendItem(t.Context(), transcript.Item{
		SessionID: sessionID, ID: "item_" + runID, RunID: runID,
		Status: transcript.ItemRunning, Kind: transcript.QuestionItem,
		Question: question, CreatedAt: parkedAt,
	}); err != nil {
		t.Fatalf("put parked transcript item: %v", err)
	}
	if err := ints.Put(t.Context(), pendingForRun(
		runID,
		sessionID,
		"proc_"+runID,
		open,
		parkedAt,
	)); err != nil {
		t.Fatalf("put parked interrupt: %v", err)
	}
	snapshot := validStoredSnapshot("proc_"+runID, core.StatusWaiting)
	snapshot.StartedAt = createdAt
	if err := processes.SaveTree(t.Context(), storedSnapshotTree(snapshot.ID, snapshot), storedCheckpoint(sessionID, storedBuildID, storedUsage())); err != nil {
		t.Fatalf("put parked process snapshot: %v", err)
	}
}

func runDraft(runID, sessionID string) execution.RunDraft {
	return execution.RunDraft{RunID: runID, SessionID: sessionID, SegmentID: "seg_open", CreatedAt: runCreatedAt}
}

// finishedRun is the terminal record a segment hands to Terminalize: the outcome
// together with the result that explains it, which is the only shape the Run row
// accepts.
func finishedRun(runID, sessionID string, outcome execution.Outcome) transcript.Run {
	state, _ := execution.Running.Terminate(outcome)
	run := transcript.Run{
		SessionID: sessionID, ID: runID, State: state, Outcome: &outcome,
		Metrics:   transcript.RunMetrics{Steps: 1},
		CreatedAt: runCreatedAt, FinishedAt: time.Unix(9, 0).UTC(),
		UpdatedAt: time.Unix(9, 0).UTC(),
	}
	if outcome == execution.OutcomeError {
		run.Error = &transcript.Problem{Kind: transcript.InternalProblem, Scope: transcript.RunProblem}
	}
	return run
}

// parkedRun is the record a park hands to Suspend: the run moving to Interrupted,
// carrying the interrupt it is parked on and what it consumed getting there.
func parkedRun(runID, sessionID string) transcript.Run {
	return transcript.Run{
		SessionID: sessionID, ID: runID, State: execution.Interrupted,
		Interrupts: []transcript.Interrupt{{
			ItemID: "itm_" + runID, RunID: runID, Kind: execution.QuestionInterrupt,
			Question: &transcript.Question{Prompt: "continue?"},
		}},
		Metrics:     transcript.RunMetrics{Steps: 1},
		CreatedAt:   runCreatedAt,
		UpdatedAt:   time.Unix(5, 0).UTC(),
		MessageMark: transcript.UnknownMessageMark,
	}
}

func pendingForRun(
	runID string,
	sessionID string,
	processID string,
	values []transcript.Interrupt,
	createdAt time.Time,
) interrupts.Pending {
	copied := slices.Clone(values)
	bindings := make([]interrupts.SuspensionBinding, len(copied))
	for index := range copied {
		copied[index].RunID = runID
		bindings[index] = interrupts.SuspensionBinding{
			InterruptItemID: copied[index].ItemID,
			ProcessID:       processID,
			SuspensionID:    "suspension_" + copied[index].ItemID,
		}
	}
	return interrupts.Pending{
		RootRunID:   runID,
		SessionID:   sessionID,
		TurnID:      "turn_" + runID,
		Interrupts:  copied,
		Suspensions: bindings,
		Continuations: []interrupts.Continuation{{
			RunID:        runID,
			ProcessID:    processID,
			RunCreatedAt: runCreatedAt,
		}},
		CreatedAt: createdAt,
	}
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
	runStore, ints := sqlite.NewRunStore(db), sqlite.NewInterruptStore(db)
	ctx := context.Background()

	if err := runStore.Admit(ctx, runDraft("run_1", "ses_A")); err != nil {
		t.Fatalf("admit: %v", err)
	}

	// park writes through the transaction's context so both statements join the
	// SAME connection (conn(ctx)); using the outer ctx would open a second
	// connection under MaxOpenConns(1) and deadlock.
	park := func(ctx context.Context) error {
		if err := ints.Put(ctx, pendingForRun(
			"run_1",
			"ses_A",
			"proc_1",
			parkedRun("run_1", "ses_A").Interrupts,
			time.Unix(2, 0).UTC(),
		)); err != nil {
			return err
		}
		return runStore.Suspend(ctx, parkedRun("run_1", "ses_A"))
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
	if err := runStore.Resume(ctx, "ses_A", execution.RunResumeDraft{RunID: "run_1", SegmentID: "seg_next"}, time.Now().UTC()); err == nil {
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
//
// It is the admission boundary's evidence for session_has_at_most_one_open_run.
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
	if err := store.Terminalize(ctx, finishedRun("run_1", "ses_A", execution.OutcomeCompleted)); err != nil {
		t.Fatalf("terminalize: %v", err)
	}
	if err := store.Admit(ctx, runDraft("run_4", "ses_A")); err != nil {
		t.Fatalf("re-admit after terminal: %v", err)
	}
}

// TestRunAdmitSharesOneRootAdmissionAcrossTheTree proves the durable B1
// foundation: one Session admits one non-terminal root tree, not one row. Child
// and grandchild Runs may execute under that root, retain their immutable
// topology, and materialize the root-owned protocol profile without storing an
// independently mutable copy.
func TestRunAdmitSharesOneRootAdmissionAcrossTheTree(t *testing.T) {
	ctx := t.Context()
	store, _ := newRunStores(t)
	profile := execution.RunProtocolProfile{RequiredFeatures: []string{"subagents"}}

	root := runDraft("run_root", "ses_A")
	root.ProtocolProfile = profile
	if err := store.Admit(ctx, root); err != nil {
		t.Fatalf("admit root: %v", err)
	}
	child := runDraft("run_child", "ses_A")
	child.SpawnedByItemID = "item_root_task"
	child.ParentRunID = "run_root"
	child.RootRunID = "run_root"
	if err := store.Admit(ctx, child); err != nil {
		t.Fatalf("admit child: %v", err)
	}
	grandchild := runDraft("run_grandchild", "ses_A")
	grandchild.SpawnedByItemID = "item_child_task"
	grandchild.ParentRunID = "run_child"
	grandchild.RootRunID = "run_root"
	if err := store.Admit(ctx, grandchild); err != nil {
		t.Fatalf("admit grandchild: %v", err)
	}
	if err := store.Admit(ctx, runDraft("run_other_root", "ses_A")); !errors.Is(err, execution.ErrSessionBusy) {
		t.Fatalf("admit second root = %v, want ErrSessionBusy", err)
	}

	for _, want := range []struct {
		id       string
		parentID string
	}{
		{id: "run_child", parentID: "run_root"},
		{id: "run_grandchild", parentID: "run_child"},
	} {
		run, found, err := store.Run(ctx, want.id)
		if err != nil || !found {
			t.Fatalf("read %s: found=%v err=%v", want.id, found, err)
		}
		if run.ParentRunID != want.parentID ||
			run.RootRunID != "run_root" ||
			!slices.Equal(run.ProtocolProfile.RequiredFeatures, profile.RequiredFeatures) {
			t.Fatalf("run %s = %+v, want parent %s, root run_root, inherited profile", want.id, run, want.parentID)
		}
	}

	tree, err := store.RunTree(ctx, "run_grandchild")
	if err != nil {
		t.Fatalf("read tree: %v", err)
	}
	treeIDs := make([]string, len(tree))
	for index, run := range tree {
		treeIDs[index] = run.ID
	}
	slices.Sort(treeIDs)
	if want := []string{"run_child", "run_grandchild", "run_root"}; !slices.Equal(treeIDs, want) {
		t.Fatalf("tree Run IDs = %v, want %v", treeIDs, want)
	}
	if other, err := store.RunTree(ctx, "run_other_root"); err != nil || len(other) != 0 {
		t.Fatalf("unadmitted tree = (%+v, %v), want empty", other, err)
	}
}

func TestRunAdmitRejectsAChildOutsideItsDurableTree(t *testing.T) {
	ctx := t.Context()
	store, _ := newRunStores(t)
	if err := store.Admit(ctx, runDraft("run_root_a", "ses_A")); err != nil {
		t.Fatalf("admit root A: %v", err)
	}
	if err := store.Admit(ctx, runDraft("run_root_b", "ses_B")); err != nil {
		t.Fatalf("admit root B: %v", err)
	}

	tests := []struct {
		name  string
		draft execution.RunDraft
		want  string
	}{
		{
			name: "missing parent",
			draft: execution.RunDraft{
				RunID: "run_child_missing", SessionID: "ses_A", SegmentID: "seg_open",
				SpawnedByItemID: "item_spawn", ParentRunID: "run_missing", RootRunID: "run_root_a",
				CreatedAt: runCreatedAt,
			},
			want: "does not exist",
		},
		{
			name: "cross session root",
			draft: execution.RunDraft{
				RunID: "run_child_cross", SessionID: "ses_A", SegmentID: "seg_open",
				SpawnedByItemID: "item_spawn", ParentRunID: "run_root_a", RootRunID: "run_root_b",
				CreatedAt: runCreatedAt,
			},
			want: "belongs to session",
		},
		{
			name: "child-owned profile",
			draft: execution.RunDraft{
				RunID: "run_child_profile", SessionID: "ses_A", SegmentID: "seg_open",
				SpawnedByItemID: "item_spawn", ParentRunID: "run_root_a", RootRunID: "run_root_a",
				ProtocolProfile: execution.RunProtocolProfile{RequiredFeatures: []string{"subagents"}},
				CreatedAt:       runCreatedAt,
			},
			want: "protocol profile is owned by root",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := store.Admit(ctx, test.draft); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Admit error = %v, want containing %q", err, test.want)
			}
		})
	}
}

// TestTerminalizeRequiresExactLiveRun pins strict lifecycle ownership: an
// unknown, mismatched, or already-terminal run is an error, never a session-
// scoped no-op that can hide a duplicated terminal decision.
func TestTerminalizeRequiresExactLiveRun(t *testing.T) {
	ctx := context.Background()
	store, _ := newRunStores(t)

	if err := store.Terminalize(ctx, finishedRun("run_unknown", "ses_unknown", execution.OutcomeCompleted)); err == nil {
		t.Fatal("terminalize unknown run must fail")
	}
	if err := store.Admit(ctx, runDraft("run_1", "ses_A")); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := store.Terminalize(ctx, finishedRun("run_other", "ses_A", execution.OutcomeCompleted)); err == nil {
		t.Fatal("terminalize mismatched run must fail")
	}
	if err := store.Terminalize(ctx, finishedRun("run_1", "ses_A", execution.OutcomeCompleted)); err != nil {
		t.Fatalf("terminalize: %v", err)
	}
	if err := store.Terminalize(ctx, finishedRun("run_1", "ses_A", execution.OutcomeCompleted)); err == nil {
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
	if err := store.Suspend(ctx, parkedRun("run_1", "ses_A")); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	// A parked run cannot complete/error/cap out without resuming — the illegal
	// transition is surfaced, not silently applied.
	if err := store.Terminalize(ctx, finishedRun("run_1", "ses_A", execution.OutcomeCompleted)); err == nil {
		t.Fatal("terminalize(completed) of a parked run must be rejected as illegal")
	}
	// The row is untouched — still non-terminal, still busy.
	if err := store.Admit(ctx, runDraft("run_2", "ses_A")); !errors.Is(err, execution.ErrSessionBusy) {
		t.Fatalf("admit after rejected terminalize = %v, want ErrSessionBusy (row untouched)", err)
	}
	// Cancellation of the same parked run is legal (Interrupted → Canceled).
	if err := store.Terminalize(ctx, finishedRun("run_1", "ses_A", execution.OutcomeCanceled)); err != nil {
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
	if err := store.Suspend(ctx, parkedRun("run_1", "ses_A")); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if err := store.Admit(ctx, runDraft("run_2", "ses_A")); !errors.Is(err, execution.ErrSessionBusy) {
		t.Fatalf("admit while suspended = %v, want ErrSessionBusy (row still non-terminal)", err)
	}
	// Resume: back to running, no second row admitted.
	if err := store.Resume(ctx, "ses_A", execution.RunResumeDraft{RunID: "run_1", SegmentID: "seg_next"}, time.Now().UTC()); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if err := store.Admit(ctx, runDraft("run_3", "ses_A")); !errors.Is(err, execution.ErrSessionBusy) {
		t.Fatalf("admit while resumed = %v, want ErrSessionBusy", err)
	}
	// Terminal frees the one reused slot.
	if err := store.Terminalize(ctx, finishedRun("run_1", "ses_A", execution.OutcomeCompleted)); err != nil {
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
	if err := store.Suspend(ctx, parkedRun("run_orphan", "ses_orphan")); err != nil {
		t.Fatalf("suspend orphan: %v", err)
	}
	// Genuinely parked: interrupted state WITH an open interrupt record.
	if err := store.Admit(ctx, runDraft("run_park", "ses_park")); err != nil {
		t.Fatalf("admit park: %v", err)
	}
	if err := store.Suspend(ctx, parkedRun("run_park", "ses_park")); err != nil {
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
	if err := store.Admit(ctx, runDraft("run_park", "ses_park")); err != nil {
		t.Fatalf("admit park: %v", err)
	}
	if err := store.Suspend(ctx, parkedRun("run_park", "ses_park")); err != nil {
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

func TestReconcileOrphansPreservesCompleteParkedRunTree(t *testing.T) {
	store, ints, transcripts, processes := newRunRecoveryStores(t)
	ctx := t.Context()
	rootID := "run_root"
	childID := "run_child"
	sessionID := "ses_tree"
	childLineage := execution.RunLineage{
		SpawnedByItemID: "item_spawn_child",
		ParentRunID:     rootID,
		RootRunID:       rootID,
	}
	if err := store.Admit(ctx, runDraft(rootID, sessionID)); err != nil {
		t.Fatalf("admit root: %v", err)
	}
	childDraft := runDraft(childID, sessionID)
	childDraft.SpawnedByItemID = childLineage.SpawnedByItemID
	childDraft.ParentRunID = childLineage.ParentRunID
	childDraft.RootRunID = childLineage.RootRunID
	if err := store.Admit(ctx, childDraft); err != nil {
		t.Fatalf("admit child: %v", err)
	}

	question := &transcript.Question{Prompt: "Continue?"}
	open := transcript.Interrupt{
		ItemID:   "item_question",
		RunID:    childID,
		Kind:     execution.QuestionInterrupt,
		Question: question,
	}
	child := parkedRun(childID, sessionID)
	child.SpawnedByItemID = childLineage.SpawnedByItemID
	child.ParentRunID = childLineage.ParentRunID
	child.RootRunID = childLineage.RootRunID
	child.Interrupts = []transcript.Interrupt{open}
	root := parkedRun(rootID, sessionID)
	root.Interrupts = nil
	if err := store.Suspend(ctx, child); err != nil {
		t.Fatalf("suspend child: %v", err)
	}
	if err := store.Suspend(ctx, root); err != nil {
		t.Fatalf("suspend root: %v", err)
	}
	if err := transcripts.AppendItem(ctx, transcript.Item{
		SessionID: sessionID,
		ID:        open.ItemID,
		RunID:     childID,
		Status:    transcript.ItemRunning,
		Kind:      transcript.QuestionItem,
		Question:  question,
		CreatedAt: time.Unix(2, 0).UTC(),
	}); err != nil {
		t.Fatalf("append child question Item: %v", err)
	}
	pending := interrupts.Pending{
		RootRunID:  rootID,
		SessionID:  sessionID,
		TurnID:     "turn_tree",
		Interrupts: []transcript.Interrupt{open},
		Suspensions: []interrupts.SuspensionBinding{{
			InterruptItemID: open.ItemID,
			ProcessID:       "process_child",
			SuspensionID:    "suspension_child",
		}},
		Continuations: []interrupts.Continuation{
			{
				RunID:           childID,
				ProcessID:       "process_child",
				ParentProcessID: "process_root",
				SpawnCallID:     "spawn_child",
				Lineage:         childLineage,
				RunCreatedAt:    runCreatedAt,
			},
			{
				RunID:        rootID,
				ProcessID:    "process_root",
				RunCreatedAt: runCreatedAt,
			},
		},
		CreatedAt: time.Unix(2, 0).UTC(),
	}
	if err := ints.Put(ctx, pending); err != nil {
		t.Fatalf("put tree Pending: %v", err)
	}
	rootSnapshot := validStoredSnapshot("process_root", core.StatusWaiting)
	rootSnapshot.StartedAt = runCreatedAt
	childSnapshot := validStoredSnapshot("process_child", core.StatusWaiting)
	childSnapshot.ParentID = rootSnapshot.ID
	childSnapshot.StartedAt = runCreatedAt
	if err := processes.SaveTree(
		ctx,
		storedSnapshotTree(rootSnapshot.ID, rootSnapshot, childSnapshot),
		storedCheckpoint("", storedBuildID, storedUsage()),
	); err != nil {
		t.Fatalf("save process tree: %v", err)
	}

	recovered, err := store.ReconcileOrphans(ctx, acceptProcessSnapshot)
	if err != nil || recovered != 0 {
		t.Fatalf("reconcile parked tree = (%d, %v), want (0, nil)", recovered, err)
	}
	for _, runID := range []string{childID, rootID} {
		run, found, err := store.Run(ctx, runID)
		if err != nil || !found || run.State != execution.Interrupted {
			t.Fatalf(
				"Run %q after reconciliation = found:%t value:%+v err:%v, want interrupted",
				runID,
				found,
				run,
				err,
			)
		}
	}
	if stored, found, err := ints.Get(ctx, rootID); err != nil ||
		!found ||
		stored.RootRunID != pending.RootRunID {
		t.Fatalf(
			"Pending after reconciliation = found:%t value:%+v err:%v, want preserved",
			found,
			stored,
			err,
		)
	}
}

func TestReconcileOrphansTerminalizesParkWhoseProcessSnapshotIsMissing(t *testing.T) {
	store, ints, transcripts, processes := newRunRecoveryStores(t)
	ctx := t.Context()
	if err := store.Admit(ctx, runDraft("run_park", "ses_park")); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := store.Suspend(ctx, parkedRun("run_park", "ses_park")); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	putParkedState(t, transcripts, ints, processes, "run_park", "ses_park")
	if err := processes.DeleteTrees(ctx, []string{"proc_run_park"}); err != nil {
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
	runs, err := store.ListRuns(ctx, "ses_park")
	if err != nil || len(runs) != 1 || runs[0].State != execution.Failed || runs[0].Error == nil || runs[0].Error.Kind != transcript.RunLostProblem {
		t.Fatalf("runs after recovery = (%+v, %v), want failed run_lost", runs, err)
	}
	if err := store.Admit(ctx, runDraft("run_next", "ses_park")); err != nil {
		t.Fatalf("admit after lost park recovery: %v", err)
	}
}

// TestReconcileOrphansRepairsWholeDurableLifecycle is the recovery boundary's
// evidence for terminal_run_explains_how_it_ended: the boot sweep does not merely mark
// an abandoned run terminal, it lands the run_lost result explaining why — the only
// chance to, since the executor that could have said is gone.
func TestReconcileOrphansRepairsWholeDurableLifecycle(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	runStore := sqlite.NewRunStore(db)
	transcripts := sqlite.NewTranscriptStore(db)

	if err := runStore.Admit(ctx, runDraft("run_lost", "ses_lost")); err != nil {
		t.Fatalf("admit lost run: %v", err)
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
	items, err := transcripts.List(ctx, "ses_lost")
	if err != nil {
		t.Fatalf("list transcript: %v", err)
	}
	runs, err := runStore.ListRuns(ctx, "ses_lost")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 || runs[0].State != execution.Failed || runs[0].Outcome == nil || *runs[0].Outcome != execution.OutcomeError {
		t.Fatalf("recovered run = %+v, want failed/error", runs)
	}
	if runs[0].Error == nil || runs[0].Error.Kind != transcript.RunLostProblem {
		t.Fatalf("recovered run failure = %+v, want run-lost problem", runs[0].Error)
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
	store := sqlite.NewRunStore(db)
	interruptStore := sqlite.NewInterruptStore(db)
	processStore := sqlite.NewProcessStore(db)
	ctx := t.Context()
	if err := store.Admit(ctx, runDraft("run_lost", "ses_1")); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := processStore.SaveTree(ctx, storedSnapshotTree(
		"proc_stale",
		validStoredSnapshot("proc_stale", core.StatusWaiting),
	), storedCheckpoint("ses_1", storedBuildID, storedUsage())); err != nil {
		t.Fatalf("put stale process snapshot: %v", err)
	}
	stale := []transcript.Interrupt{{
		ItemID: "item_stale", RunID: "run_stale",
		Kind: execution.QuestionInterrupt, Question: &transcript.Question{Prompt: "continue?"},
	}}
	if err := interruptStore.Put(ctx, pendingForRun(
		"run_stale",
		"ses_1",
		"proc_stale",
		stale,
		time.Unix(2, 0).UTC(),
	)); err != nil {
		t.Fatalf("put stale interrupt: %v", err)
	}
	if swept, err := store.ReconcileOrphans(ctx, acceptProcessSnapshot); err != nil || swept != 1 {
		t.Fatalf("reconcile = (%d, %v), want (1, nil)", swept, err)
	}
	if pending, err := interruptStore.List(ctx, "ses_1"); err != nil || len(pending) != 0 {
		t.Fatalf("stale interrupts after reconcile = (%+v, %v), want none", pending, err)
	}
	if _, _, err := processStore.LoadTree(ctx, "proc_stale"); !errors.Is(err, execution.ErrProcessSnapshotNotFound) {
		t.Fatalf("stale process snapshot after reconcile = %v, want not found", err)
	}
}

// TestReconcileOrphansRejectsPartialParkWithoutMutatingIt is the recovery boundary's
// evidence for parked_tree_has_exactly_one_open_interrupt_set: a barrier missing
// half its durable set is neither resumed nor half-repaired, because a sweep that
// "fixed" only part of it would leave the whole tree waiting forever.
func TestReconcileOrphansRejectsPartialParkWithoutMutatingIt(t *testing.T) {
	store, ints, _, _ := newRunRecoveryStores(t)
	ctx := t.Context()
	if err := store.Admit(ctx, runDraft("run_park", "ses_park")); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := store.Suspend(ctx, parkedRun("run_park", "ses_park")); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	parkedAt := time.Unix(2, 0).UTC()
	question := &transcript.Question{Prompt: "Continue?"}
	open := []transcript.Interrupt{{ItemID: "item_missing", RunID: "run_park", Kind: execution.QuestionInterrupt, Question: question}}
	if err := ints.Put(ctx, pendingForRun(
		"run_park",
		"ses_park",
		"proc_park",
		open,
		parkedAt,
	)); err != nil {
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
	if err := store.Suspend(ctx, parkedRun("run_park", "ses_park")); err != nil {
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
	if err := store.Admit(ctx, execution.RunDraft{SegmentID: "seg_open",
		RunID: "run_park", SessionID: "ses_park", ModelSelection: testModelSelection(t, "openai", "gpt-test"), CreatedAt: time.Unix(0, 0),
	}); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := store.Suspend(ctx, parkedRun("run_park", "ses_park")); err != nil {
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
	if err := store.Suspend(ctx, parkedRun("run_park", "ses_park")); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	putParkedState(t, transcripts, ints, processes, "run_park", "ses_park")
	pending, found, err := ints.Get(ctx, "run_park")
	if err != nil || !found {
		t.Fatalf("get pending: found=%v err=%v", found, err)
	}
	root, _ := pending.RootContinuation()
	root.DrainedTools = []interrupts.DrainedTool{{
		ItemID: "item_run_park", CallID: "call_run_park", Name: "ask_user", Arguments: "{}",
	}}
	pending.Continuations[0] = root
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
	if err := store.Suspend(ctx, parkedRun("run_park", "ses_park")); err != nil {
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
	if _, _, err := processes.LoadTree(ctx, "proc_run_park"); !errors.Is(err, execution.ErrProcessSnapshotNotFound) {
		t.Fatalf("snapshot after incompatible recovery = %v, want not found", err)
	}
	runs, err := store.ListRuns(ctx, "ses_park")
	if err != nil || len(runs) != 1 || runs[0].Error == nil || runs[0].Error.Kind != transcript.RunLostProblem {
		t.Fatalf("runs after incompatible recovery = (%+v, %v), want run_lost", runs, err)
	}
}

func TestReconcileOrphansRejectsSnapshotValidatorFailureWithoutMutation(t *testing.T) {
	store, ints, transcripts, processes := newRunRecoveryStores(t)
	ctx := t.Context()
	if err := store.Admit(ctx, runDraft("run_park", "ses_park")); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := store.Suspend(ctx, parkedRun("run_park", "ses_park")); err != nil {
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
	if err := store.Terminalize(ctx, finishedRun("run_1", "ses_1", execution.Outcome(255))); err == nil {
		t.Fatal("terminalize accepted an unknown outcome")
	}
}

// TestPageRunsReturnsEveryLifecyclePosition pins what the run page means on the
// durable record: the whole history, with each row's position readable. A parked
// run is non-terminal — it still holds its session's admission slot — but it is
// waiting on a person rather than executing, and a filter has to be able to tell
// those two apart. The live in-process registry answers none of this after a
// restart, which is why the read lives here.
func TestPageRunsReturnsEveryLifecyclePosition(t *testing.T) {
	ctx := context.Background()
	store, ints := newRunStores(t)

	for _, draft := range []execution.RunDraft{
		{RunID: "run_live", SessionID: "ses_A", SegmentID: "seg_open", CreatedAt: time.Unix(0, 20)},
		{RunID: "run_parked", SessionID: "ses_B", SegmentID: "seg_open", CreatedAt: time.Unix(0, 10)},
		{RunID: "run_done", SessionID: "ses_C", SegmentID: "seg_open", CreatedAt: time.Unix(0, 30)},
	} {
		if err := store.Admit(ctx, draft); err != nil {
			t.Fatalf("admit %s: %v", draft.RunID, err)
		}
	}
	parked := parkedRun("run_parked", "ses_B")
	if err := ints.Put(ctx, pendingForRun(
		parked.ID,
		parked.SessionID,
		"proc_parked",
		parked.Interrupts,
		time.Unix(0, 10).UTC(),
	)); err != nil {
		t.Fatalf("put interrupt: %v", err)
	}
	if err := store.Suspend(ctx, parked); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if err := store.Terminalize(ctx, finishedRun("run_done", "ses_C", execution.OutcomeCompleted)); err != nil {
		t.Fatalf("terminalize: %v", err)
	}

	all, err := store.PageRuns(ctx, "", nil, false, 0, "", 0)
	if err != nil {
		t.Fatalf("page runs: %v", err)
	}
	if got := runIDs(all); len(got) != 3 {
		t.Fatalf("unfiltered page = %v, want all three positions", got)
	}

	for _, tt := range []struct {
		name     string
		statuses []execution.RunStatus
		want     []string
	}{
		{"running only", []execution.RunStatus{execution.StatusRunning}, []string{"run_live"}},
		{"waiting only", []execution.RunStatus{execution.StatusWaiting}, []string{"run_parked"}},
		{"finished only", []execution.RunStatus{execution.StatusFinished}, []string{"run_done"}},
		{"recovery pair", []execution.RunStatus{execution.StatusRunning, execution.StatusWaiting}, []string{"run_live", "run_parked"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rows, err := store.PageRuns(ctx, "", tt.statuses, false, 0, "", 0)
			if err != nil {
				t.Fatalf("page runs: %v", err)
			}
			got := runIDs(rows)
			slices.Sort(got)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("page = %v, want %v", got, tt.want)
			}
		})
	}

	// The session filter is independent of the status one: scoping to a session that
	// holds a parked run still finds it.
	if scoped, err := store.PageRuns(ctx, "ses_B", nil, false, 0, "", 0); err != nil || len(scoped) != 1 || scoped[0].ID != "run_parked" {
		t.Fatalf("ses_B scoped = %+v (err %v), want the parked run", scoped, err)
	}
	if _, err := store.PageRuns(ctx, "", []execution.RunStatus{execution.RunStatus(9)}, false, 0, "", 0); err == nil {
		t.Fatal("page runs accepted an unknown status instead of refusing to widen the page")
	}
}

// TestPageRunsOrdersNewestFirst keeps the page stable across calls AND fixes its
// direction: an unordered scan lets SQLite pick, and a cursor page over an unstable
// order skips and repeats rows. Newest first is the contract's order because the run
// a client is looking for is almost always the last one.
func TestPageRunsOrdersNewestFirst(t *testing.T) {
	ctx := context.Background()
	store, _ := newRunStores(t)

	for _, draft := range []execution.RunDraft{
		{RunID: "run_c", SessionID: "ses_C", SegmentID: "seg_open", CreatedAt: time.Unix(0, 30)},
		{RunID: "run_a", SessionID: "ses_A", SegmentID: "seg_open", CreatedAt: time.Unix(0, 10)},
		{RunID: "run_b", SessionID: "ses_B", SegmentID: "seg_open", CreatedAt: time.Unix(0, 20)},
	} {
		if err := store.Admit(ctx, draft); err != nil {
			t.Fatalf("admit %s: %v", draft.RunID, err)
		}
	}
	rows, err := store.PageRuns(ctx, "", nil, false, 0, "", 0)
	if err != nil {
		t.Fatalf("page runs: %v", err)
	}
	if order := runIDs(rows); !slices.Equal(order, []string{"run_c", "run_b", "run_a"}) {
		t.Fatalf("order = %v, want newest admission first", order)
	}
}

// TestPageRunsSeeksBeforeItsAnchor bounds the page in the query: continuing from a
// position must skip exactly the rows already returned, including a row that shares
// its admission nanosecond with the anchor.
func TestPageRunsSeeksBeforeItsAnchor(t *testing.T) {
	ctx := context.Background()
	store, _ := newRunStores(t)

	for _, draft := range []execution.RunDraft{
		{RunID: "run_a", SessionID: "ses_A", SegmentID: "seg_open", CreatedAt: time.Unix(0, 10)},
		{RunID: "run_b", SessionID: "ses_B", SegmentID: "seg_open", CreatedAt: time.Unix(0, 20)},
		{RunID: "run_c", SessionID: "ses_C", SegmentID: "seg_open", CreatedAt: time.Unix(0, 20)},
	} {
		if err := store.Admit(ctx, draft); err != nil {
			t.Fatalf("admit %s: %v", draft.RunID, err)
		}
	}

	first, err := store.PageRuns(ctx, "", nil, false, 0, "", 2)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if order := runIDs(first); !slices.Equal(order, []string{"run_c", "run_b"}) {
		t.Fatalf("first page = %v, want run_c then run_b", order)
	}

	// run_b shares run_c's admission time, so a time-only bound would drop it or
	// repeat it; the run id breaks the tie.
	rest, err := store.PageRuns(ctx, "", nil, false, first[1].CreatedAt.UnixNano(), first[1].ID, 2)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if order := runIDs(rest); !slices.Equal(order, []string{"run_a"}) {
		t.Fatalf("second page = %v, want only run_a", order)
	}
}

// TestPageRunsSelectsRootsOrDescendants separates the default browse page from the
// explicit tree-aware page, while runs.get resolves ANY run id.
//
// The page's predicate is "has no spawning item", not "this build makes no
// children": the moment child runs exist, a default page that had been returning
// everything would start mixing subtree rows into a root listing.
func TestPageRunsSelectsRootsOrDescendants(t *testing.T) {
	ctx := context.Background()
	store, _ := newRunStores(t)

	root := finishedRun("run_root", "ses_A", execution.OutcomeCompleted)
	root.CreatedAt = time.Unix(0, 10).UTC()
	child := finishedRun("run_child", "ses_A", execution.OutcomeCompleted)
	child.CreatedAt = time.Unix(0, 20).UTC()
	child.SpawnedByItemID = "it_spawn"
	child.ParentRunID = "run_root"
	child.RootRunID = "run_root"
	grandchild := finishedRun("run_grandchild", "ses_A", execution.OutcomeCompleted)
	grandchild.CreatedAt = time.Unix(0, 30).UTC()
	grandchild.SpawnedByItemID = "it_spawn_grandchild"
	grandchild.ParentRunID = child.ID
	grandchild.RootRunID = root.ID
	for _, run := range []transcript.Run{root, child, grandchild} {
		if err := store.Restore(ctx, run); err != nil {
			t.Fatalf("restore %s: %v", run.ID, err)
		}
	}

	page, err := store.PageRuns(ctx, "", nil, false, 0, "", 0)
	if err != nil {
		t.Fatalf("page runs: %v", err)
	}
	if order := runIDs(page); !slices.Equal(order, []string{"run_root"}) {
		t.Fatalf("page = %v, want the root run only", order)
	}
	treePage, err := store.PageRuns(ctx, "", nil, true, 0, "", 0)
	if err != nil {
		t.Fatalf("page runs with descendants: %v", err)
	}
	if order := runIDs(treePage); !slices.Equal(order, []string{"run_grandchild", "run_child", "run_root"}) {
		t.Fatalf("page with descendants = %v, want one stable newest-first order", order)
	}

	found, ok, err := store.Run(ctx, "run_child")
	if err != nil || !ok {
		t.Fatalf("read child run: ok=%v err=%v", ok, err)
	}
	if found.SpawnedByItemID != "it_spawn" ||
		found.ParentRunID != "run_root" ||
		found.RootRunID != "run_root" ||
		found.SessionID != "ses_A" {
		t.Fatalf("child = %+v, want its complete lineage and session", found)
	}
	if _, ok, err := store.Run(ctx, "run_absent"); err != nil || ok {
		t.Fatalf("absent run: ok=%v err=%v, want a clean miss", ok, err)
	}

	lineage, err := store.RunsWithAncestors(ctx, []string{grandchild.ID})
	if err != nil {
		t.Fatalf("read grandchild lineage: %v", err)
	}
	if got := runIDs(lineage); !slices.Equal(got, []string{grandchild.ID, child.ID, root.ID}) {
		t.Fatalf("grandchild lineage = %v, want grandchild through root", got)
	}
}

func TestPageRunTreeItemsUsesDurableParentEdges(t *testing.T) {
	store, _, transcripts, _ := newRunRecoveryStores(t)
	ctx := t.Context()
	root := finishedRun("run_root", "ses_A", execution.OutcomeCompleted)
	child := finishedRun("run_child", "ses_A", execution.OutcomeCompleted)
	child.SpawnedByItemID = "item_spawn_child"
	child.ParentRunID = root.ID
	child.RootRunID = root.ID
	grandchild := finishedRun("run_grandchild", "ses_A", execution.OutcomeCompleted)
	grandchild.SpawnedByItemID = "item_spawn_grandchild"
	grandchild.ParentRunID = child.ID
	grandchild.RootRunID = root.ID
	sibling := finishedRun("run_sibling", "ses_A", execution.OutcomeCompleted)
	sibling.SpawnedByItemID = "item_spawn_sibling"
	sibling.ParentRunID = root.ID
	sibling.RootRunID = root.ID
	for _, run := range []transcript.Run{root, child, grandchild, sibling} {
		if err := store.Restore(ctx, run); err != nil {
			t.Fatalf("restore %s: %v", run.ID, err)
		}
	}
	for index, runID := range []string{root.ID, child.ID, grandchild.ID, sibling.ID} {
		if err := transcripts.AppendItem(ctx, transcript.Item{
			SessionID: root.SessionID,
			ID:        "item_" + runID,
			RunID:     runID,
			Status:    transcript.ItemCompleted,
			Kind:      transcript.UserMessage,
			CreatedAt: time.Unix(0, int64(index+1)).UTC(),
		}); err != nil {
			t.Fatalf("append %s item: %v", runID, err)
		}
	}

	rows, err := transcripts.PageRunTreeItems(ctx, child.ID, transcript.OldestFirst, 0, 0)
	if err != nil {
		t.Fatalf("page child subtree items: %v", err)
	}
	var got []string
	for _, row := range rows {
		got = append(got, row.Item.RunID)
	}
	if !slices.Equal(got, []string{child.ID, grandchild.ID}) {
		t.Fatalf("child subtree items = %v, want child and grandchild only", got)
	}

	tail, err := transcripts.PageRunTreeItems(ctx, root.ID, transcript.NewestFirst, 0, 2)
	if err != nil {
		t.Fatalf("page root subtree tail: %v", err)
	}
	got = got[:0]
	for _, row := range tail {
		got = append(got, row.Item.RunID)
	}
	if !slices.Equal(got, []string{sibling.ID, grandchild.ID}) {
		t.Fatalf("root subtree tail = %v, want newest two items", got)
	}
}

func runIDs(runs []transcript.Run) []string {
	out := make([]string, 0, len(runs))
	for _, run := range runs {
		out = append(out, run.ID)
	}
	return out
}

// TestRunProtocolProfileIsImmutable proves the invariant
// `run_protocol_profile_is_immutable` at the runs.admission boundary: the
// admission INSERT is the profile's only writer, so parking, resuming and
// terminalizing the run all read back the contract it was admitted under.
//
// It is checked at the store rather than above it because that is where the
// guarantee lives: no statement other than the INSERT names the column, and a
// later transition that started writing one would break this and nothing else.
func TestRunProtocolProfileIsImmutable(t *testing.T) {
	ctx := context.Background()
	store, interruptStore := newRunStores(t)

	admitted := execution.RunProtocolProfile{
		RequiredFeatures: []string{"subagents"},
		InterruptKinds:   []execution.InterruptKind{execution.ApprovalInterrupt, execution.QuestionInterrupt},
	}
	draft := runDraft("run_1", "ses_A")
	draft.ProtocolProfile = admitted
	if err := store.Admit(ctx, draft); err != nil {
		t.Fatalf("admit: %v", err)
	}

	// A park hands the store a whole Run record, including a profile — the store
	// must ignore it rather than let the segment restate the contract.
	parked := parkedRun("run_1", "ses_A")
	parked.ProtocolProfile = execution.RunProtocolProfile{}
	pending := pendingForRun(
		"run_1",
		"ses_A",
		"proc_1",
		parked.Interrupts,
		time.Unix(5, 0).UTC(),
	)
	pending.ProtocolProfile = admitted
	if err := interruptStore.Put(ctx, pending); err != nil {
		t.Fatalf("put interrupt: %v", err)
	}
	if err := store.Suspend(ctx, parked); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	assertRunProfile(t, store, "run_1", admitted, "after park")

	// The park's own row carries the profile onward, which is where a continuation
	// reads it: the resume never sees the runs row before it reopens the segment.
	pending, found, err := interruptStore.Get(ctx, "run_1")
	if err != nil || !found {
		t.Fatalf("get interrupt: %v (found=%v)", err, found)
	}
	if !slices.Equal(pending.ProtocolProfile.RequiredFeatures, admitted.RequiredFeatures) ||
		!slices.Equal(pending.ProtocolProfile.InterruptKinds, admitted.InterruptKinds) {
		t.Fatalf("park hand-off profile = %v, want %v", pending.ProtocolProfile, admitted)
	}

	if err := store.Resume(ctx, "ses_A", execution.RunResumeDraft{RunID: "run_1", SegmentID: "seg_next"}, time.Now().UTC()); err != nil {
		t.Fatalf("resume: %v", err)
	}
	assertRunProfile(t, store, "run_1", admitted, "after resume")

	finished := finishedRun("run_1", "ses_A", execution.OutcomeCompleted)
	finished.ProtocolProfile = execution.RunProtocolProfile{}
	if err := store.Terminalize(ctx, finished); err != nil {
		t.Fatalf("terminalize: %v", err)
	}
	assertRunProfile(t, store, "run_1", admitted, "after terminal")
}

func assertRunProfile(t *testing.T, store *sqlite.RunStore, runID string, want execution.RunProtocolProfile, when string) {
	t.Helper()
	runs, err := store.ListRuns(context.Background(), "ses_A")
	if err != nil {
		t.Fatalf("list runs %s: %v", when, err)
	}
	for _, run := range runs {
		if run.ID != runID {
			continue
		}
		if !slices.Equal(run.ProtocolProfile.RequiredFeatures, want.RequiredFeatures) ||
			!slices.Equal(run.ProtocolProfile.InterruptKinds, want.InterruptKinds) {
			t.Fatalf("profile %s = %v, want %v", when, run.ProtocolProfile, want)
		}
		return
	}
	t.Fatalf("run %q missing %s", runID, when)
}
