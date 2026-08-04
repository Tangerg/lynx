package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

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

func newRunProjectionStores(t *testing.T) (*sqlite.RunStore, *sqlite.InterruptStore, *sqlite.TranscriptStore) {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewRunStore(db), sqlite.NewInterruptStore(db), sqlite.NewTranscriptStore(db)
}

// runCreatedAt is when every fixture Run is admitted. The interrupt record
// carries the same instant, and a park whose two records disagree about when its
// Run started is rejected as an incomplete boundary.
var runCreatedAt = time.Unix(1, 0).UTC()

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
			ItemID: "itm_" + runID, ItemOccurredAt: runCreatedAt,
			RunID: runID, Kind: execution.QuestionInterrupt,
			Question: &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "continue?"}}},
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
		if copied[index].ItemOccurredAt.IsZero() {
			copied[index].ItemOccurredAt = createdAt
		}
		bindings[index] = interrupts.SuspensionBinding{
			InterruptItemID: copied[index].ItemID,
			ProcessID:       processID,
			SuspensionID:    "suspension_" + copied[index].ItemID,
		}
	}
	return interrupts.Pending{
		RootRunID:    runID,
		SessionID:    sessionID,
		ExecutorID:   "turn_" + runID,
		Interrupts:   copied,
		Suspensions:  bindings,
		Capabilities: capabilitiesForInterrupts(copied),
		Continuations: []interrupts.Continuation{{
			RunID:        runID,
			ProcessID:    processID,
			RunCreatedAt: runCreatedAt,
		}},
		CreatedAt: createdAt,
	}
}

func capabilitiesForInterrupts(values []transcript.Interrupt) execution.RunCapabilities {
	capabilities := execution.RunCapabilities{}
	for _, value := range values {
		capabilities.InterruptKinds = append(capabilities.InterruptKinds, value.Kind)
	}
	return capabilities.Normalized()
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
		if err := ints.Open(ctx, pendingForRun(
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
// topology, and materialize the root-owned run capabilities without storing an
// independently mutable copy.
func TestRunAdmitSharesOneRootAdmissionAcrossTheTree(t *testing.T) {
	ctx := t.Context()
	store, _ := newRunStores(t)
	capabilities := execution.RunCapabilities{ChildRuns: true}

	root := runDraft("run_root", "ses_A")
	root.Capabilities = capabilities
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
			run.Capabilities.ChildRuns != capabilities.ChildRuns {
			t.Fatalf("run %s = %+v, want parent %s, root run_root, inherited capabilities", want.id, run, want.parentID)
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
			name: "child-owned capabilities",
			draft: execution.RunDraft{
				RunID: "run_child_capabilities", SessionID: "ses_A", SegmentID: "seg_open",
				SpawnedByItemID: "item_spawn", ParentRunID: "run_root_a", RootRunID: "run_root_a",
				Capabilities: execution.RunCapabilities{ChildRuns: true},
				CreatedAt:    runCreatedAt,
			},
			want: "capabilities are owned by root",
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
	if err := ints.Open(ctx, pendingForRun(
		parked.ID,
		parked.SessionID,
		"proc_parked",
		parked.Interrupts,
		time.Unix(0, 10).UTC(),
	)); err != nil {
		t.Fatalf("open interrupt: %v", err)
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
	store, _, transcripts := newRunProjectionStores(t)
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
			SessionID:  root.SessionID,
			ID:         "item_" + runID,
			RunID:      runID,
			Status:     transcript.ItemCompleted,
			Kind:       transcript.UserMessage,
			OccurredAt: time.Unix(0, int64(index+1)).UTC(),
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

// TestRunCapabilitiesAreImmutable proves the invariant
// `run_capabilities_are_immutable` at the runs.admission boundary: the
// admission INSERT is the capability set's only writer, so parking, resuming and
// terminalizing the Run all read back the capabilities admitted with it.
//
// It is checked at the store rather than above it because that is where the
// guarantee lives: no statement other than the INSERT names the column, and a
// later transition that started writing one would break this and nothing else.
func TestRunCapabilitiesAreImmutable(t *testing.T) {
	ctx := context.Background()
	store, interruptStore := newRunStores(t)

	admitted := execution.RunCapabilities{
		ChildRuns:      true,
		InterruptKinds: []execution.InterruptKind{execution.ApprovalInterrupt, execution.QuestionInterrupt},
	}
	draft := runDraft("run_1", "ses_A")
	draft.Capabilities = admitted
	if err := store.Admit(ctx, draft); err != nil {
		t.Fatalf("admit: %v", err)
	}

	// A park hands the store a whole Run record, including capabilities — the store
	// must ignore it rather than let the segment restate the value.
	parked := parkedRun("run_1", "ses_A")
	parked.Capabilities = execution.RunCapabilities{}
	pending := pendingForRun(
		"run_1",
		"ses_A",
		"proc_1",
		parked.Interrupts,
		time.Unix(5, 0).UTC(),
	)
	pending.Capabilities = admitted
	if err := interruptStore.Open(ctx, pending); err != nil {
		t.Fatalf("open interrupt: %v", err)
	}
	if err := store.Suspend(ctx, parked); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	assertRunCapabilities(t, store, "run_1", admitted, "after park")

	// The Pending hand-off carries capabilities onward for the continuation: resume
	// never reads a replacement from the request before reopening the segment.
	pending, found, err := interruptStore.Get(ctx, "run_1")
	if err != nil || !found {
		t.Fatalf("get interrupt: %v (found=%v)", err, found)
	}
	if pending.Capabilities.ChildRuns != admitted.ChildRuns ||
		!slices.Equal(pending.Capabilities.InterruptKinds, admitted.InterruptKinds) {
		t.Fatalf("park hand-off capabilities = %v, want %v", pending.Capabilities, admitted)
	}

	if err := store.Resume(ctx, "ses_A", execution.RunResumeDraft{RunID: "run_1", SegmentID: "seg_next"}, time.Now().UTC()); err != nil {
		t.Fatalf("resume: %v", err)
	}
	assertRunCapabilities(t, store, "run_1", admitted, "after resume")

	finished := finishedRun("run_1", "ses_A", execution.OutcomeCompleted)
	finished.Capabilities = execution.RunCapabilities{}
	if err := store.Terminalize(ctx, finished); err != nil {
		t.Fatalf("terminalize: %v", err)
	}
	assertRunCapabilities(t, store, "run_1", admitted, "after terminal")
}

func assertRunCapabilities(t *testing.T, store *sqlite.RunStore, runID string, want execution.RunCapabilities, when string) {
	t.Helper()
	runs, err := store.ListRuns(context.Background(), "ses_A")
	if err != nil {
		t.Fatalf("list runs %s: %v", when, err)
	}
	for _, run := range runs {
		if run.ID != runID {
			continue
		}
		if run.Capabilities.ChildRuns != want.ChildRuns ||
			!slices.Equal(run.Capabilities.InterruptKinds, want.InterruptKinds) {
			t.Fatalf("capabilities %s = %v, want %v", when, run.Capabilities, want)
		}
		return
	}
	t.Fatalf("run %q missing %s", runID, when)
}
