package bootstrap

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/lynx/app/runtime/internal/application/conversations"
	runsapp "github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	sqlite "github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
	runfixture "github.com/Tangerg/lynx/app/runtime/internal/testsupport/runfixture"
)

const bootstrapCheckpointBuildID = "test-build"

// bootstrapCheckpoint deliberately treats the executor payload as opaque. These
// Bootstrap fixtures verify Application write-sets, not Agent Framework wire;
// only adapter/agentexec may construct or interpret a tree snapshot.
func bootstrapCheckpoint(rootMemberID, sessionID string) runsapp.ExecutorCheckpoint {
	return runsapp.ExecutorCheckpoint{
		RootMemberID: rootMemberID,
		Payload:      []byte("opaque executor checkpoint"),
		BuildID:      bootstrapCheckpointBuildID,
		Scope:        runsapp.ExecutionScope{SessionID: sessionID},
	}
}

func bootstrapPending(
	runID, sessionID, memberID, itemID string,
	runCreatedAt, barrierCreatedAt time.Time,
) runsapp.Pending {
	question := &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?"}}}
	return runsapp.Pending{
		RootRunID:  runID,
		SessionID:  sessionID,
		ExecutorID: "turn_" + runID,
		Capabilities: run.Capabilities{
			InterruptKinds: []interrupt.Kind{interrupt.Question},
		},
		Interrupts: []transcript.Interrupt{{
			ItemID: itemID, ItemOccurredAt: parkCreatedAt,
			RunID:    runID,
			Kind:     interrupt.Question,
			Question: question,
		}},
		Bindings: []runsapp.InterruptBinding{{
			InterruptItemID: itemID,
			MemberID:        memberID,
			RequestID:       "request-" + memberID,
		}},
		Continuations: []runsapp.Continuation{{
			RunID:        runID,
			MemberID:     memberID,
			RunCreatedAt: runCreatedAt,
		}},
		CreatedAt: barrierCreatedAt,
	}
}

// sessionStores keeps the real durable collaborators visible to this integration
// fixture while delegating every tested write-set to the persistence adapter.
type sessionStores struct {
	*persistence.SessionStores
	sessions    *sqlite.SessionStore
	transcript  *sqlite.TranscriptStore
	interrupts  *persistence.InterruptStore
	runs        *sqlite.RunStore
	checkpoints *persistence.ExecutorCheckpointStore
	history     *conversations.Messages
	plan        *sqlite.PlanStore
	approvals   *sqlite.ApprovalRuleStore
	goals       *sqlite.GoalStore
}

// newWriteSetFixture builds the persistence adapter over a fresh sqlite
// database so the atomic write-sets run against the real stores + transactor.
func newWriteSetFixture(t *testing.T) (sessionStores, *sqlite.RunStore, *persistence.InterruptStore) {
	t.Helper()
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	runs := sqlite.NewRunStore(db)
	ints := persistence.NewInterruptStore(sqlite.NewInterruptStore(db))
	plan := sqlite.NewPlanStore(db)
	approvals := sqlite.NewApprovalRuleStore(db)
	ss := sessionStores{
		sessions:    sqlite.NewSessionStore(db),
		transcript:  sqlite.NewTranscriptStore(db),
		interrupts:  ints,
		runs:        runs,
		checkpoints: persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db)),
		history:     conversations.NewMessages(sqlite.NewMessageStore(db)),
		plan:        plan,
		approvals:   approvals,
		goals:       sqlite.NewGoalStore(db),
	}
	ss.SessionStores = persistence.NewSessionStores(persistence.SessionStoresConfig{
		Sessions: ss.sessions, Transcript: ss.transcript, Interrupts: ss.interrupts,
		Runs: ss.runs, ExecutorCheckpoints: ss.checkpoints, History: ss.history, Plan: ss.plan,
		Approvals: ss.approvals, Goals: ss.goals,
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlite.RunInTx(ctx, db, fn)
		},
	})
	return ss, runs, ints
}

// parkCreatedAt is when the fixture's parked Run was admitted.
var parkCreatedAt = time.Unix(1, 0).UTC()

// restoredRun is a finished Run as an import carries it — the only shape a
// restore accepts, since a Run that has not ended cannot be replayed into a
// session from the outside.
func restoredRun(sessionID, runID string, at time.Time) run.Run {
	outcome := run.OutcomeCompleted
	return runfixture.MustRestore(run.Snapshot{SessionID: sessionID, ID: runID, State: run.Completed,
		Outcome:   &outcome,
		CreatedAt: at, FinishedAt: at, UpdatedAt: at, MessageMark: 0})

}

func parkSessionARootRun(
	t *testing.T,
	sessions *sqlite.SessionStore,
	runs *sqlite.RunStore,
	ints *persistence.InterruptStore,
	checkpoints *persistence.ExecutorCheckpointStore,
) string {
	return parkWithGoalLease(t, sessions, runs, ints, checkpoints, "ses_A", "run_1", "")
}

func parkWithGoalLease(
	t *testing.T,
	sessions *sqlite.SessionStore,
	runs *sqlite.RunStore,
	ints *persistence.InterruptStore,
	checkpoints *persistence.ExecutorCheckpointStore,
	sessionID, runID, goalLeaseID string,
) string {
	t.Helper()
	ctx := context.Background()
	startedAt := time.Unix(0, 0).UTC()
	if _, err := sessions.Ensure(ctx, session.Session{
		ID:        sessionID,
		StartedAt: startedAt,
		UpdatedAt: startedAt,
	}); err != nil {
		t.Fatalf("ensure session: %v", err)
	}
	memberID := "member_" + runID
	checkpoint := bootstrapCheckpoint(memberID, sessionID)
	checkpoint.Scope.GoalLeaseID = goalLeaseID
	if err := checkpoints.SaveCheckpoint(ctx, checkpoint); err != nil {
		t.Fatalf("save executor checkpoint: %v", err)
	}
	if err := runs.Admit(ctx, run.Draft{
		RunID: runID, SessionID: sessionID, SegmentID: "seg_open",
		GoalLeaseID: goalLeaseID,
		Capabilities: run.Capabilities{
			InterruptKinds: []interrupt.Kind{interrupt.Question},
		},
		CreatedAt: parkCreatedAt,
	}); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := runs.Suspend(ctx, runfixture.MustRestore(run.Snapshot{SessionID: sessionID, ID: runID, State: run.Waiting,
		GoalLeaseID: goalLeaseID,
		Capabilities: run.Capabilities{
			InterruptKinds: []interrupt.Kind{interrupt.Question},
		},

		CreatedAt:   parkCreatedAt,
		MessageMark: run.UnknownMessageMark}),
	); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	pending := bootstrapPending(
		runID,
		sessionID,
		memberID,
		"item_"+runID,
		parkCreatedAt,
		time.Unix(0, 0).UTC(),
	)
	pending.GoalLeaseID = goalLeaseID
	if err := ints.Open(ctx, pending); err != nil {
		t.Fatalf("open interrupt: %v", err)
	}
	return memberID
}

// TestApplyTerminalDropsInterruptAndTerminalizes: abandoning a parked run frees both
// the resumable record and the durable admission slot, atomically.
func TestApplyTerminalDropsInterruptAndTerminalizes(t *testing.T) {
	ss, runs, ints := newWriteSetFixture(t)
	ctx := t.Context()
	memberID := parkSessionARootRun(t, ss.sessions, runs, ints, ss.checkpoints)
	if err := ss.checkpoints.SaveCheckpoint(ctx, bootstrapCheckpoint(memberID, "ses_A")); err != nil {
		t.Fatalf("replace root executor checkpoint: %v", err)
	}
	finishedAt := time.Date(2026, 7, 13, 2, 3, 4, 0, time.UTC)
	parked, found, err := runs.Run(ctx, "run_1")
	if err != nil || !found {
		t.Fatalf("read parked Run: found=%t err=%v", found, err)
	}
	terminal, err := parked.CancelWaiting("", finishedAt, 0)
	if err != nil {
		t.Fatalf("cancel parked Run: %v", err)
	}

	if err := ss.ApplyTerminal(ctx, sessions.TerminalPlan{CheckpointRootID: memberID, Runs: []run.Run{terminal}}); err != nil {
		t.Fatalf("ApplyTerminal: %v", err)
	}
	if open, _ := ints.List(ctx, "ses_A"); len(open) != 0 {
		t.Fatalf("interrupt survived cancel: %+v", open)
	}
	if _, err := ss.checkpoints.LoadCheckpoint(ctx, memberID); !errors.Is(err, runsapp.ErrExecutorCheckpointNotFound) {
		t.Fatalf("executor checkpoint after cancel = %v, want not found", err)
	}
	// The admission row is terminal, so the session can start a fresh run.
	if err := runs.Admit(ctx, run.Draft{RunID: "run_2", SessionID: "ses_A", SegmentID: "seg_open", CreatedAt: parkCreatedAt.Add(time.Minute)}); err != nil {
		t.Fatalf("admit after cancel = %v, want the slot freed", err)
	}
	storedRuns, err := runs.ListRuns(ctx, "ses_A")
	if err != nil || len(storedRuns) != 2 || storedRuns[0].State() != run.Canceled {
		t.Fatalf("terminal runs = %+v (err %v), want the canceled run and its successor", storedRuns, err)
	}
}

func TestApplyTerminalRecoversLostParkAtomically(t *testing.T) {
	ss, runs, ints := newWriteSetFixture(t)
	ctx := t.Context()
	memberID := parkSessionARootRun(t, ss.sessions, runs, ints, ss.checkpoints)
	if err := ss.checkpoints.SaveCheckpoint(ctx, bootstrapCheckpoint(memberID, "ses_A")); err != nil {
		t.Fatalf("replace root executor checkpoint: %v", err)
	}
	finishedAt := time.Date(2026, 7, 16, 2, 3, 4, 0, time.UTC)
	parked, found, err := runs.Run(ctx, "run_1")
	if err != nil || !found {
		t.Fatalf("read parked Run: found=%t err=%v", found, err)
	}
	terminal, err := parked.RecoverLost(run.Failure{Kind: run.FailureLost}, finishedAt, 0)
	if err != nil {
		t.Fatalf("recover parked Run: %v", err)
	}

	if err := ss.ApplyTerminal(ctx, sessions.TerminalPlan{CheckpointRootID: memberID, Runs: []run.Run{terminal}}); err != nil {
		t.Fatalf("ApplyTerminal run_lost: %v", err)
	}
	if open, _ := ints.List(ctx, "ses_A"); len(open) != 0 {
		t.Fatalf("interrupt survived run_lost: %+v", open)
	}
	if _, err := ss.checkpoints.LoadCheckpoint(ctx, memberID); !errors.Is(err, runsapp.ErrExecutorCheckpointNotFound) {
		t.Fatalf("executor checkpoint after run_lost = %v, want not found", err)
	}
	if err := runs.Admit(ctx, run.Draft{RunID: "run_2", SessionID: "ses_A", SegmentID: "seg_open", CreatedAt: parkCreatedAt.Add(time.Minute)}); err != nil {
		t.Fatalf("admit after run_lost = %v, want the slot freed", err)
	}
	storedRuns, err := runs.ListRuns(ctx, "ses_A")
	failure, failed := run.Failure{}, false
	if len(storedRuns) > 0 {
		failure, failed = storedRuns[0].Failure()
	}
	if err != nil || len(storedRuns) != 2 || !failed || failure.Kind != run.FailureLost {
		t.Fatalf("terminal runs = %+v (err %v), want run_lost", storedRuns, err)
	}
}

func TestApplyTerminalChargesGoalOwnedParkAtomically(t *testing.T) {
	ss, runs, ints := newWriteSetFixture(t)
	ctx := t.Context()
	const leaseID = "lease_terminal_park"
	memberID := parkWithGoalLease(
		t,
		ss.sessions,
		runs,
		ints,
		ss.checkpoints,
		"ses_A",
		"run_goal",
		leaseID,
	)
	goalValue, err := goal.New(
		"ses_A",
		"finish the parked run",
		modelref.Selection{},
		goal.Budget{},
		leaseID,
		parkCreatedAt,
	)
	if err != nil {
		t.Fatalf("new Goal: %v", err)
	}
	if _, applied, err := ss.goals.Save(ctx, goalValue, goal.Version{}); err != nil || !applied {
		t.Fatalf("save Goal: applied=%t err=%v", applied, err)
	}

	costUSD := 0.75
	outcome := run.OutcomeLost
	finishedAt := parkCreatedAt.Add(time.Minute)
	parked, found, err := runs.Run(ctx, "run_goal")
	if err != nil || !found {
		t.Fatalf("read parked Goal Run: found=%t err=%v", found, err)
	}
	parked, err = parked.AdvanceMetrics(runfixture.MustMetrics(runfixture.MetricsInput{Steps: 4,
		Usage: &accounting.Usage{Total: accounting.Totals{CostUSD: &costUSD}}}), finishedAt)
	if err != nil {
		t.Fatalf("advance parked Goal Run metrics: %v", err)
	}
	terminal, err := parked.RecoverLost(run.Failure{Kind: run.FailureLost}, finishedAt, 0)
	if err != nil {
		t.Fatalf("recover parked Goal Run: %v", err)
	}

	goalRun := goal.RunRecord{
		SessionID: "ses_A", LeaseID: leaseID, RunID: terminal.ID(),
		Outcome: outcome, CostUSD: costUSD, Steps: 4, CompletedAt: finishedAt,
	}
	if err := ss.ApplyTerminal(ctx, sessions.TerminalPlan{
		Runs: []run.Run{terminal}, CheckpointRootID: memberID, GoalRun: &goalRun,
	}); err != nil {
		t.Fatalf("ApplyTerminal: %v", err)
	}

	storedGoal, found, err := ss.goals.Get(ctx, "ses_A")
	if err != nil || !found {
		t.Fatalf("get Goal: found=%t err=%v", found, err)
	}
	if storedGoal.Used != (goal.Usage{Runs: 1, CostUSD: costUSD, Steps: 4}) ||
		storedGoal.Status != goal.StatusPaused ||
		storedGoal.Reason.Code != goal.ReasonRunNotCompleted {
		t.Fatalf("Goal after terminal park = %+v", storedGoal)
	}
	storedRun, found, err := runs.Run(ctx, terminal.ID())
	if err != nil || !found || storedRun.State() != run.Failed {
		t.Fatalf("Run after terminal park = found:%t value:%+v err:%v", found, storedRun, err)
	}
	if open, err := ints.List(ctx, "ses_A"); err != nil || len(open) != 0 {
		t.Fatalf("interrupt after terminal park = %+v err=%v", open, err)
	}
}

// TestApplyRollbackDropsRunsAndFreesAdmission: a rollback that abandons a parked
// run drops its interrupt and its Run record — which is also how the session's
// admission slot is released.
//
// It is the rollback boundary's evidence for two invariants:
// dropped_run_leaves_nothing_behind and goal_never_outlives_its_session.
func TestApplyRollbackDropsRunsAndFreesAdmission(t *testing.T) {
	ss, runs, ints := newWriteSetFixture(t)
	ctx := context.Background()
	memberID := parkSessionARootRun(t, ss.sessions, runs, ints, ss.checkpoints)
	if err := ss.plan.Replace(ctx, "ses_A", []plan.Step{{Description: "future work", Status: plan.StatusPending}}); err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	seedGoal(t, ss, "ses_A")

	if err := ss.ApplyRollback(ctx, sessions.RollbackPlan{
		SessionID:         "ses_A",
		KeepMessageMark:   -1,
		DropRunIDs:        []string{"run_1"},
		CheckpointRootIDs: []string{memberID},
	}); err != nil {
		t.Fatalf("ApplyRollback: %v", err)
	}
	if _, ok, err := ss.goals.Get(ctx, "ses_A"); err != nil || ok {
		t.Fatalf("goal survived the rollback: ok=%v err=%v", ok, err)
	}
	if open, _ := ints.List(ctx, "ses_A"); len(open) != 0 {
		t.Fatalf("dropped run's interrupt survived rollback: %+v", open)
	}
	if _, err := ss.checkpoints.LoadCheckpoint(ctx, memberID); !errors.Is(err, runsapp.ErrExecutorCheckpointNotFound) {
		t.Fatalf("executor checkpoint after rollback = %v, want not found", err)
	}
	if err := runs.Admit(ctx, run.Draft{RunID: "run_2", SessionID: "ses_A", SegmentID: "seg_open", CreatedAt: parkCreatedAt.Add(time.Minute)}); err != nil {
		t.Fatalf("admit after rollback = %v, want the slot freed", err)
	}
	// The rollback carries no recorded boundary, so the plan is left exactly as it
	// was: this runtime does not know what the plan held at that moment, and clearing
	// it would be a guess dressed as a restore.
	if got, err := ss.plan.List(ctx, "ses_A"); err != nil || len(got) != 1 {
		t.Fatalf("plan after rollback = %+v, %v, want the live list untouched", got, err)
	}
}

// TestApplyRollbackRepublishesBoundaryPlan: a rollback publishes the list the
// boundary recorded as a NEW state commit. The revision has to move forward —
// under a lower one, a client that already folded a later list ignores the
// rollback as stale and keeps showing work the session no longer has.
//
// This is state_revision_never_goes_backwards at its hardest point: the VALUE goes
// backwards here by design, which is exactly when the temptation to move the
// revision with it appears.
func TestApplyRollbackRepublishesBoundaryPlan(t *testing.T) {
	ss, runs, ints := newWriteSetFixture(t)
	ctx := t.Context()
	memberID := parkSessionARootRun(t, ss.sessions, runs, ints, ss.checkpoints)
	if err := ss.plan.Replace(ctx, "ses_A", []plan.Step{
		{Description: "work the rollback discards", Status: plan.StatusInProgress},
	}); err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	before, err := ss.plan.State(ctx, "ses_A")
	if err != nil {
		t.Fatalf("read plan: %v", err)
	}

	boundary := []plan.Step{{Description: "the plan at the boundary", Status: plan.StatusPending}}
	if err := ss.ApplyRollback(ctx, sessions.RollbackPlan{
		SessionID:         "ses_A",
		KeepMessageMark:   -1,
		DropRunIDs:        []string{"run_1"},
		CheckpointRootIDs: []string{memberID},
		Plan:              sessions.PlanBoundary{Steps: boundary, Recorded: true},
	}); err != nil {
		t.Fatalf("ApplyRollback: %v", err)
	}

	after, err := ss.plan.State(ctx, "ses_A")
	if err != nil {
		t.Fatalf("read plan: %v", err)
	}
	if len(after.Steps) != 1 || after.Steps[0].Description != "the plan at the boundary" {
		t.Fatalf("plan after rollback = %+v, want the boundary plan", after.Steps)
	}
	if after.Revision <= before.Revision {
		t.Fatalf("revision after rollback = %d, want greater than %d", after.Revision, before.Revision)
	}
}

// TestApplyRollbackClearsToARecordedEmptyBoundary: a boundary that recorded an
// empty list clears the live one — and still through a forward revision, because
// "the list was empty here" is a value, not an absence.
func TestApplyRollbackClearsToARecordedEmptyBoundary(t *testing.T) {
	ss, runs, ints := newWriteSetFixture(t)
	ctx := t.Context()
	memberID := parkSessionARootRun(t, ss.sessions, runs, ints, ss.checkpoints)
	if err := ss.plan.Replace(ctx, "ses_A", []plan.Step{{Description: "later work", Status: plan.StatusPending}}); err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	before, err := ss.plan.State(ctx, "ses_A")
	if err != nil {
		t.Fatalf("read plan: %v", err)
	}

	if err := ss.ApplyRollback(ctx, sessions.RollbackPlan{
		SessionID:         "ses_A",
		KeepMessageMark:   -1,
		DropRunIDs:        []string{"run_1"},
		CheckpointRootIDs: []string{memberID},
		Plan:              sessions.PlanBoundary{Recorded: true},
	}); err != nil {
		t.Fatalf("ApplyRollback: %v", err)
	}

	after, err := ss.plan.State(ctx, "ses_A")
	if err != nil {
		t.Fatalf("read plan: %v", err)
	}
	if len(after.Steps) != 0 {
		t.Fatalf("plan after rollback = %+v, want cleared to the recorded empty boundary", after.Steps)
	}
	if after.Revision <= before.Revision {
		t.Fatalf("revision after rollback = %d, want greater than %d", after.Revision, before.Revision)
	}
}

// TestApplyForkBranchesAndSeeds: fork branches a child, seeds its chat log with
// the resolved prefix, and titles it — all in one transaction (the child's Fork
// joins the seed + rename rather than opening its own connection).
func TestApplyForkBranchesAndSeeds(t *testing.T) {
	ss, _, _ := newWriteSetFixture(t)
	ctx := context.Background()
	parent, err := ss.sessions.Create(ctx, "parent", "/repo")
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}

	child, err := ss.ApplyFork(ctx, sessions.ForkPlan{
		ParentID: parent.ID,
		Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("hello"))},
		Plan:     []plan.Step{{Description: "inherited plan", Status: plan.StatusInProgress}},
		Title:    "Child",
	})
	if err != nil {
		t.Fatalf("ApplyFork: %v", err)
	}
	if child.ID == "" || child.ID == parent.ID {
		t.Fatalf("child id = %q (parent %q)", child.ID, parent.ID)
	}
	if child.Title != "Child" {
		t.Fatalf("child title = %q, want Child", child.Title)
	}
	msgs, err := ss.history.Read(ctx, child.ID)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("child history = %d (err %v), want 1 seeded message", len(msgs), err)
	}
	// The branch inherits the plan the copied conversation was following, and the
	// parent's own list is untouched by its child being created.
	got, err := ss.plan.List(ctx, child.ID)
	if err != nil || len(got) != 1 || got[0].Description != "inherited plan" {
		t.Fatalf("child plan = %+v (err %v), want the boundary list", got, err)
	}
	if got, err := ss.plan.List(ctx, parent.ID); err != nil || len(got) != 0 {
		t.Fatalf("parent plan = %+v (err %v), want none written by the fork", got, err)
	}
}

// TestApplyDeleteRemovesRunRows: the delete cascade removes the session's durable
// admission rows, so the runs table keeps no dead rows for a deleted session (and
// the slot is free).
//
// It is the delete boundary's evidence for dropped_run_leaves_nothing_behind.
func TestApplyDeleteRemovesRunRows(t *testing.T) {
	ss, runs, ints := newWriteSetFixture(t)
	ctx := context.Background()
	if err := ss.sessions.Restore(ctx, session.Session{ID: "ses_A"}); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	memberID := parkSessionARootRun(t, ss.sessions, runs, ints, ss.checkpoints)
	orphanMemberID := "member_orphan"
	if err := ss.checkpoints.SaveCheckpoint(ctx, bootstrapCheckpoint(orphanMemberID, "ses_A")); err != nil {
		t.Fatalf("seed orphan checkpoint: %v", err)
	}
	if err := ss.plan.Replace(ctx, "ses_A", []plan.Step{{Description: "owned", Status: plan.StatusPending}}); err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	if err := ss.approvals.Put(ctx, testAllowRule(t, approval.ScopeSession, "ses_A", "shell")); err != nil {
		t.Fatalf("seed approval: %v", err)
	}

	if err := ss.ApplyDelete(ctx, sessions.DeletePlan{SessionID: "ses_A"}); err != nil {
		t.Fatalf("ApplyDelete: %v", err)
	}
	if open, _ := ints.List(ctx, "ses_A"); len(open) != 0 {
		t.Fatalf("interrupt survived delete: %+v", open)
	}
	if _, err := ss.checkpoints.LoadCheckpoint(ctx, memberID); !errors.Is(err, runsapp.ErrExecutorCheckpointNotFound) {
		t.Fatalf("executor checkpoint after delete = %v, want not found", err)
	}
	if _, err := ss.checkpoints.LoadCheckpoint(ctx, orphanMemberID); !errors.Is(err, runsapp.ErrExecutorCheckpointNotFound) {
		t.Fatalf("orphan executor checkpoint after delete = %v, want not found", err)
	}
	if _, err := ss.sessions.Get(ctx, "ses_A"); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("session survived delete: %v", err)
	}
	if got, err := ss.plan.List(ctx, "ses_A"); err != nil || len(got) != 0 {
		t.Fatalf("plan after delete = %+v, %v, want cleared", got, err)
	}
	if got, err := ss.approvals.Visible(ctx, "ses_A", ""); err != nil || len(got) != 0 {
		t.Fatalf("session approvals after delete = %+v, %v, want cleared", got, err)
	}
	// The non-terminal admission row is gone (not just terminal), so a fresh admit
	// succeeds — proving the delete cascade dropped the runs rows.
	if err := runs.Admit(ctx, run.Draft{RunID: "run_2", SessionID: "ses_A", SegmentID: "seg_open", CreatedAt: parkCreatedAt.Add(time.Minute)}); err != nil {
		t.Fatalf("admit after delete = %v, want the slot freed", err)
	}
}

func TestApplyRestoreClearsSessionOwnedProjections(t *testing.T) {
	ss, _, _ := newWriteSetFixture(t)
	ctx := t.Context()
	if err := ss.sessions.Restore(ctx, session.Session{ID: "ses_A", CWD: "/repo"}); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if err := ss.plan.Replace(ctx, "ses_A", []plan.Step{{Description: "stale", Status: plan.StatusPending}}); err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	seedGoal(t, ss, "ses_A")
	sessionRule := testAllowRule(t, approval.ScopeSession, "ses_A", "shell")
	projectRule := testAllowRule(t, approval.ScopeProject, "/repo", "write")
	globalRule := testAllowRule(t, approval.ScopeGlobal, "", "read")
	for _, rule := range []approval.Rule{sessionRule, projectRule, globalRule} {
		if err := ss.approvals.Put(ctx, rule); err != nil {
			t.Fatalf("seed approval %s: %v", rule.ID, err)
		}
	}

	if err := ss.ApplyRestore(ctx, sessions.RestorePlan{Session: session.Session{ID: "ses_A", CWD: "/repo"}}); err != nil {
		t.Fatalf("ApplyRestore: %v", err)
	}
	if got, err := ss.plan.List(ctx, "ses_A"); err != nil || len(got) != 0 {
		t.Fatalf("plan after restore = %+v, %v, want cleared", got, err)
	}
	if _, ok, err := ss.goals.Get(ctx, "ses_A"); err != nil || ok {
		t.Fatalf("goal survived the restore: ok=%v err=%v", ok, err)
	}
	rules, err := ss.approvals.Visible(ctx, "ses_A", "/repo")
	if err != nil {
		t.Fatalf("visible approvals: %v", err)
	}
	ids := make(map[string]bool, len(rules))
	for _, rule := range rules {
		ids[rule.ID] = true
	}
	if ids[sessionRule.ID] || !ids[projectRule.ID] || !ids[globalRule.ID] || len(ids) != 2 {
		t.Fatalf("approvals after restore = %v, want project+global only", ids)
	}
}

func testAllowRule(t *testing.T, scope approval.Scope, scopeKey, toolName string) approval.Rule {
	t.Helper()
	rule, err := approval.NewRule(scope, scopeKey, toolName, "", approval.Allow)
	if err != nil {
		t.Fatalf("new approval rule: %v", err)
	}
	return rule
}

func TestApplyRollbackRejectsInvalidCheckpointSetAtomically(t *testing.T) {
	ss, _, _ := newWriteSetFixture(t)
	ctx := t.Context()
	parent, err := ss.sessions.Create(ctx, "parent", "/repo")
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	seedGoal(t, ss, parent.ID)
	if err := ss.checkpoints.SaveCheckpoint(ctx, bootstrapCheckpoint("member_preserve", parent.ID)); err != nil {
		t.Fatalf("seed executor checkpoint: %v", err)
	}

	err = ss.ApplyRollback(ctx, sessions.RollbackPlan{
		SessionID: parent.ID, KeepMessageMark: -1,
		CheckpointRootIDs: []string{"member_preserve", ""},
	})
	if err == nil {
		t.Fatal("ApplyRollback unexpectedly accepted an invalid checkpoint root member ID")
	}
	if _, ok, err := ss.goals.Get(ctx, parent.ID); err != nil || !ok {
		t.Fatalf("goal clear was not rolled back: ok=%v err=%v", ok, err)
	}
	if _, err := ss.checkpoints.LoadCheckpoint(ctx, "member_preserve"); err != nil {
		t.Fatalf("executor checkpoint deletion was not rolled back: %v", err)
	}
}

func TestApplyRestoreRollsBackOnRunIdentityConflict(t *testing.T) {
	ss, _, _ := newWriteSetFixture(t)
	ctx := t.Context()
	for _, ses := range []session.Session{
		{ID: "ses_A", Title: "source", CWD: "/source"},
		{ID: "ses_B", Title: "target", CWD: "/target"},
	} {
		if err := ss.sessions.Restore(ctx, ses); err != nil {
			t.Fatalf("seed session %s: %v", ses.ID, err)
		}
	}
	now := time.Now().UTC()
	if err := ss.runs.Restore(ctx, restoredRun("ses_A", "run_shared", now)); err != nil {
		t.Fatalf("seed source run: %v", err)
	}
	if err := ss.transcript.AppendItem(ctx, transcript.Item{
		SessionID: "ses_A", RunID: "run_shared", ID: "item_shared", OccurredAt: now,
	}); err != nil {
		t.Fatalf("seed source item: %v", err)
	}
	if err := ss.runs.Restore(ctx, restoredRun("ses_B", "run_target", now)); err != nil {
		t.Fatalf("seed target run: %v", err)
	}
	if err := ss.history.Seed(ctx, "ses_B", []chat.Message{chat.NewUserMessage(chat.NewTextPart("before"))}); err != nil {
		t.Fatalf("seed target history: %v", err)
	}

	err := ss.ApplyRestore(ctx, sessions.RestorePlan{
		Session:  session.Session{ID: "ses_B", Title: "replacement", CWD: "/replacement"},
		Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("after"))},
		Runs:     []run.Run{restoredRun("ses_B", "run_shared", now)},
	})
	if !errors.Is(err, run.ErrIdentityConflict) {
		t.Fatalf("ApplyRestore error = %v, want run.ErrIdentityConflict", err)
	}

	target, err := ss.sessions.Get(ctx, "ses_B")
	if err != nil || target.Title != "target" || target.CWD != "/target" {
		t.Fatalf("target session after rollback = %+v, %v", target, err)
	}
	messages, err := ss.history.Read(ctx, "ses_B")
	if err != nil || len(messages) != 1 {
		t.Fatalf("target history after rollback = %+v, %v", messages, err)
	}
	targetRuns, err := ss.runs.ListRuns(ctx, "ses_B")
	if err != nil || len(targetRuns) != 1 || targetRuns[0].ID() != "run_target" {
		t.Fatalf("target runs after rollback = %+v, %v", targetRuns, err)
	}
	sourceItems, err := ss.transcript.List(ctx, "ses_A")
	if err != nil || len(sourceItems) != 1 {
		t.Fatalf("source items after conflict = %+v, %v", sourceItems, err)
	}
	sourceRuns, err := ss.runs.ListRuns(ctx, "ses_A")
	if err != nil || len(sourceRuns) != 1 {
		t.Fatalf("source runs after conflict = %+v, %v", sourceRuns, err)
	}
}

// seedGoal stores an active goal for a session, so a write-set
// test can assert the delete/rollback/restore cascade clears it.
func seedGoal(t *testing.T, ss sessionStores, sessionID string) {
	t.Helper()
	if _, err := ss.sessions.Get(t.Context(), sessionID); errors.Is(err, session.ErrNotFound) {
		if err := ss.sessions.Restore(t.Context(), session.Session{ID: sessionID, StartedAt: time.Unix(0, 0), UpdatedAt: time.Unix(0, 0), Revision: 1}); err != nil {
			t.Fatalf("seed goal session %q: %v", sessionID, err)
		}
	} else if err != nil {
		t.Fatalf("get goal session %q: %v", sessionID, err)
	}
	g, _ := goal.New(sessionID, "obj", modelref.Selection{}, goal.Budget{}, "lease-"+sessionID, time.Unix(0, 0))
	if _, applied, err := ss.goals.Save(context.Background(), g, goal.Version{}); err != nil || !applied {
		t.Fatalf("seed goal %q: applied=%v err=%v", sessionID, applied, err)
	}
}

// TestApplyDeleteClearsSessionGoal proves a goal is part of the atomic delete
// cascade (D1): a deleted session leaves no orphan goal behind.
//
// It is the delete boundary's evidence for goal_never_outlives_its_session.
func TestApplyDeleteClearsSessionGoal(t *testing.T) {
	ss, _, _ := newWriteSetFixture(t)
	ctx := context.Background()
	seedGoal(t, ss, "ses_goal")

	if err := ss.ApplyDelete(ctx, sessions.DeletePlan{SessionID: "ses_goal"}); err != nil {
		t.Fatalf("ApplyDelete: %v", err)
	}
	if _, ok, err := ss.goals.Get(ctx, "ses_goal"); err != nil || ok {
		t.Fatalf("goal survived the delete cascade: ok=%v err=%v", ok, err)
	}
}
