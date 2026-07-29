package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/conversation"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
	sqlite "github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

const bootstrapCheckpointBuildID = "test-build"

func bootstrapWaitingSnapshot(id string) core.ProcessSnapshot {
	started := time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC)
	return core.ProcessSnapshot{
		SchemaVersion: core.ProcessSnapshotSchemaVersion,
		ID:            id,
		Deployment:    core.DeploymentRef{Name: "chat", Digest: "digest"},
		StartedAt:     started,
		Status:        core.StatusWaiting,
		Suspension: &agent.Suspension{
			SchemaVersion: agent.SuspensionSchemaVersion,
			ID:            "suspension-" + id,
			Prompt:        json.RawMessage(`"continue?"`),
			ResumeSchema:  json.RawMessage(`{"type":"boolean"}`),
			CreatedAt:     started,
		},
	}
}

func bootstrapSnapshotTree(rootID string, snapshots ...core.ProcessSnapshot) core.ProcessSnapshotTree {
	return core.ProcessSnapshotTree{
		RootID:    rootID,
		Snapshots: snapshots,
	}
}

func bootstrapCheckpoint(sessionID string, usage accounting.Snapshot) execution.ProcessCheckpoint {
	return execution.ProcessCheckpoint{
		BuildID: bootstrapCheckpointBuildID,
		Scope:   execution.TurnScope{SessionID: sessionID},
		Usage:   usage,
	}
}

// sessionStores keeps the real durable collaborators visible to this integration
// fixture while delegating every tested write-set to the persistence adapter.
type sessionStores struct {
	*persistence.SessionStores
	sessions   *sqlite.SessionStore
	transcript *sqlite.TranscriptStore
	interrupts *sqlite.InterruptStore
	runs       *sqlite.RunStore
	processes  *sqlite.ProcessStore
	history    *conversation.Messages
	todos      *sqlite.TodoStore
	approvals  *sqlite.ApprovalRuleStore
	goals      *sqlite.GoalStore
}

// newWriteSetFixture builds the persistence adapter over a fresh sqlite
// database so the atomic write-sets run against the real stores + transactor.
func newWriteSetFixture(t *testing.T) (sessionStores, *sqlite.RunStore, *sqlite.InterruptStore) {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	runs := sqlite.NewRunStore(db)
	ints := sqlite.NewInterruptStore(db)
	todos := sqlite.NewTodoStore(db)
	approvals := sqlite.NewApprovalRuleStore(db)
	ss := sessionStores{
		sessions:   sqlite.NewSessionStore(db),
		transcript: sqlite.NewTranscriptStore(db),
		interrupts: ints,
		runs:       runs,
		processes:  sqlite.NewProcessStore(db),
		history:    conversation.NewMessages(sqlite.NewMessageStore(db)),
		todos:      todos,
		approvals:  approvals,
		goals:      sqlite.NewGoalStore(db),
	}
	ss.SessionStores = persistence.NewSessionStores(persistence.SessionStoresConfig{
		Sessions: ss.sessions, Transcript: ss.transcript, Interrupts: ss.interrupts,
		Runs: ss.runs, Processes: ss.processes, History: ss.history, Todos: ss.todos,
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
func restoredRun(sessionID, runID string, at time.Time) transcript.Run {
	outcome := execution.OutcomeCompleted
	return transcript.Run{
		SessionID: sessionID, ID: runID, State: execution.Completed,
		Outcome:   &outcome,
		CreatedAt: at, FinishedAt: at, UpdatedAt: at, MessageMark: 0,
	}
}

func park(
	t *testing.T,
	sessions *sqlite.SessionStore,
	runs *sqlite.RunStore,
	ints *sqlite.InterruptStore,
	processes *sqlite.ProcessStore,
	sessionID, runID string,
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
	processID := "proc_" + runID
	if err := processes.SaveTree(ctx, bootstrapSnapshotTree(processID, bootstrapWaitingSnapshot(processID)), bootstrapCheckpoint(sessionID, accounting.Snapshot{})); err != nil {
		t.Fatalf("save process snapshot: %v", err)
	}
	if err := runs.Admit(ctx, execution.RunDraft{RunID: runID, SessionID: sessionID, SegmentID: "seg_open", CreatedAt: parkCreatedAt}); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := runs.Suspend(ctx, transcript.Run{
		SessionID: sessionID, ID: runID, State: execution.Interrupted,
		Interrupts: []transcript.Interrupt{{
			ItemID: "item_" + runID, Kind: execution.QuestionInterrupt,
			Question: &transcript.Question{Prompt: "continue?"},
		}},
		CreatedAt:   parkCreatedAt,
		MessageMark: transcript.UnknownMessageMark,
	}); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if err := ints.Put(ctx, interrupts.Pending{RunID: runID, SessionID: sessionID, ProcessID: processID, CreatedAt: time.Unix(0, 0)}); err != nil {
		t.Fatalf("put interrupt: %v", err)
	}
	return processID
}

// TestApplyTerminalDropsInterruptAndTerminalizes: abandoning a parked run frees both
// the resumable record and the durable admission slot, atomically.
func TestApplyTerminalDropsInterruptAndTerminalizes(t *testing.T) {
	ss, runs, ints := newWriteSetFixture(t)
	ctx := t.Context()
	processID := park(t, ss.sessions, runs, ints, ss.processes, "ses_A", "run_1")
	child := bootstrapWaitingSnapshot("child_" + processID)
	child.ParentID = processID
	if err := ss.processes.SaveTree(ctx, bootstrapSnapshotTree(
		processID,
		bootstrapWaitingSnapshot(processID),
		child,
	), bootstrapCheckpoint("ses_A", accounting.Snapshot{})); err != nil {
		t.Fatalf("save child process snapshot: %v", err)
	}
	outcome := execution.OutcomeCanceled
	finishedAt := time.Date(2026, 7, 13, 2, 3, 4, 0, time.UTC)

	if err := ss.ApplyTerminal(ctx, sessions.TerminalPlan{ProcessID: processID, Run: transcript.Run{
		SessionID: "ses_A", ID: "run_1", State: execution.Canceled,
		Outcome: &outcome, CreatedAt: parkCreatedAt,
		FinishedAt: finishedAt, UpdatedAt: finishedAt, MessageMark: 0,
	}}); err != nil {
		t.Fatalf("ApplyTerminal: %v", err)
	}
	if open, _ := ints.List(ctx, "ses_A"); len(open) != 0 {
		t.Fatalf("interrupt survived cancel: %+v", open)
	}
	if _, _, err := ss.processes.LoadTree(ctx, processID); !errors.Is(err, execution.ErrProcessSnapshotNotFound) {
		t.Fatalf("process snapshot after cancel = %v, want not found", err)
	}
	if _, _, err := ss.processes.LoadTree(ctx, child.ID); !errors.Is(err, execution.ErrProcessSnapshotNotFound) {
		t.Fatalf("child process snapshot after cancel = %v, want not found", err)
	}
	// The admission row is terminal, so the session can start a fresh run.
	if err := runs.Admit(ctx, execution.RunDraft{RunID: "run_2", SessionID: "ses_A", SegmentID: "seg_open", CreatedAt: parkCreatedAt.Add(time.Minute)}); err != nil {
		t.Fatalf("admit after cancel = %v, want the slot freed", err)
	}
	storedRuns, err := runs.ListRuns(ctx, "ses_A")
	if err != nil || len(storedRuns) != 2 || storedRuns[0].State != execution.Canceled {
		t.Fatalf("terminal runs = %+v (err %v), want the canceled run and its successor", storedRuns, err)
	}
}

func TestApplyTerminalRecoversLostParkAtomically(t *testing.T) {
	ss, runs, ints := newWriteSetFixture(t)
	ctx := t.Context()
	processID := park(t, ss.sessions, runs, ints, ss.processes, "ses_A", "run_1")
	child := bootstrapWaitingSnapshot("child_" + processID)
	child.ParentID = processID
	if err := ss.processes.SaveTree(ctx, bootstrapSnapshotTree(
		processID,
		bootstrapWaitingSnapshot(processID),
		child,
	), bootstrapCheckpoint("ses_A", accounting.Snapshot{})); err != nil {
		t.Fatalf("save child process snapshot: %v", err)
	}
	outcome := execution.OutcomeError
	finishedAt := time.Date(2026, 7, 16, 2, 3, 4, 0, time.UTC)

	if err := ss.ApplyTerminal(ctx, sessions.TerminalPlan{ProcessID: processID, Run: transcript.Run{
		SessionID: "ses_A", ID: "run_1", State: execution.Failed,
		Outcome: &outcome, Error: &transcript.Problem{
			Kind: transcript.RunLostProblem, Scope: transcript.RunProblem,
		},
		CreatedAt:  parkCreatedAt,
		FinishedAt: finishedAt, UpdatedAt: finishedAt, MessageMark: 0,
	}}); err != nil {
		t.Fatalf("ApplyTerminal run_lost: %v", err)
	}
	if open, _ := ints.List(ctx, "ses_A"); len(open) != 0 {
		t.Fatalf("interrupt survived run_lost: %+v", open)
	}
	if _, _, err := ss.processes.LoadTree(ctx, processID); !errors.Is(err, execution.ErrProcessSnapshotNotFound) {
		t.Fatalf("process snapshot after run_lost = %v, want not found", err)
	}
	if _, _, err := ss.processes.LoadTree(ctx, child.ID); !errors.Is(err, execution.ErrProcessSnapshotNotFound) {
		t.Fatalf("child process snapshot after run_lost = %v, want not found", err)
	}
	if err := runs.Admit(ctx, execution.RunDraft{RunID: "run_2", SessionID: "ses_A", SegmentID: "seg_open", CreatedAt: parkCreatedAt.Add(time.Minute)}); err != nil {
		t.Fatalf("admit after run_lost = %v, want the slot freed", err)
	}
	storedRuns, err := runs.ListRuns(ctx, "ses_A")
	if err != nil || len(storedRuns) != 2 || storedRuns[0].Error == nil ||
		storedRuns[0].Error.Kind != transcript.RunLostProblem {
		t.Fatalf("terminal runs = %+v (err %v), want run_lost", storedRuns, err)
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
	processID := park(t, ss.sessions, runs, ints, ss.processes, "ses_A", "run_1")
	if err := ss.todos.Replace(ctx, "ses_A", []todo.Item{{Content: "future work", Status: todo.StatusPending}}); err != nil {
		t.Fatalf("seed todos: %v", err)
	}
	seedGoal(t, ss, "ses_A")

	if err := ss.ApplyRollback(ctx, sessions.RollbackPlan{
		SessionID:  "ses_A",
		KeepMark:   -1,
		DropRunIDs: []string{"run_1"},
		ProcessIDs: []string{processID},
	}); err != nil {
		t.Fatalf("ApplyRollback: %v", err)
	}
	if _, ok, err := ss.goals.Get(ctx, "ses_A"); err != nil || ok {
		t.Fatalf("goal survived the rollback: ok=%v err=%v", ok, err)
	}
	if open, _ := ints.List(ctx, "ses_A"); len(open) != 0 {
		t.Fatalf("dropped run's interrupt survived rollback: %+v", open)
	}
	if _, _, err := ss.processes.LoadTree(ctx, processID); !errors.Is(err, execution.ErrProcessSnapshotNotFound) {
		t.Fatalf("process snapshot after rollback = %v, want not found", err)
	}
	if err := runs.Admit(ctx, execution.RunDraft{RunID: "run_2", SessionID: "ses_A", SegmentID: "seg_open", CreatedAt: parkCreatedAt.Add(time.Minute)}); err != nil {
		t.Fatalf("admit after rollback = %v, want the slot freed", err)
	}
	if got, err := ss.todos.List(ctx, "ses_A"); err != nil || len(got) != 0 {
		t.Fatalf("todos after rollback = %+v, %v, want cleared", got, err)
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
	processID := park(t, ss.sessions, runs, ints, ss.processes, "ses_A", "run_1")
	if err := ss.todos.Replace(ctx, "ses_A", []todo.Item{{Content: "owned", Status: todo.StatusPending}}); err != nil {
		t.Fatalf("seed todos: %v", err)
	}
	if err := ss.approvals.Put(ctx, testApprovalRule(t, approval.ScopeSession, "ses_A", "shell", approval.Allow)); err != nil {
		t.Fatalf("seed approval: %v", err)
	}

	if err := ss.ApplyDelete(ctx, sessions.DeletePlan{SessionIDs: []string{"ses_A"}}); err != nil {
		t.Fatalf("ApplyDelete: %v", err)
	}
	if open, _ := ints.List(ctx, "ses_A"); len(open) != 0 {
		t.Fatalf("interrupt survived delete: %+v", open)
	}
	if _, _, err := ss.processes.LoadTree(ctx, processID); !errors.Is(err, execution.ErrProcessSnapshotNotFound) {
		t.Fatalf("process snapshot after delete = %v, want not found", err)
	}
	if _, err := ss.sessions.Get(ctx, "ses_A"); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("session survived delete: %v", err)
	}
	if got, err := ss.todos.List(ctx, "ses_A"); err != nil || len(got) != 0 {
		t.Fatalf("todos after delete = %+v, %v, want cleared", got, err)
	}
	if got, err := ss.approvals.Visible(ctx, "ses_A", ""); err != nil || len(got) != 0 {
		t.Fatalf("session approvals after delete = %+v, %v, want cleared", got, err)
	}
	// The non-terminal admission row is gone (not just terminal), so a fresh admit
	// succeeds — proving the delete cascade dropped the runs rows.
	if err := runs.Admit(ctx, execution.RunDraft{RunID: "run_2", SessionID: "ses_A", SegmentID: "seg_open", CreatedAt: parkCreatedAt.Add(time.Minute)}); err != nil {
		t.Fatalf("admit after delete = %v, want the slot freed", err)
	}
}

func TestApplyRestoreClearsSessionOwnedProjections(t *testing.T) {
	ss, _, _ := newWriteSetFixture(t)
	ctx := t.Context()
	if err := ss.sessions.Restore(ctx, session.Session{ID: "ses_A", Cwd: "/repo"}); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if err := ss.todos.Replace(ctx, "ses_A", []todo.Item{{Content: "stale", Status: todo.StatusPending}}); err != nil {
		t.Fatalf("seed todos: %v", err)
	}
	seedGoal(t, ss, "ses_A")
	sessionRule := testApprovalRule(t, approval.ScopeSession, "ses_A", "shell", approval.Allow)
	projectRule := testApprovalRule(t, approval.ScopeProject, "/repo", "write", approval.Allow)
	globalRule := testApprovalRule(t, approval.ScopeGlobal, "", "read", approval.Allow)
	for _, rule := range []approval.Rule{sessionRule, projectRule, globalRule} {
		if err := ss.approvals.Put(ctx, rule); err != nil {
			t.Fatalf("seed approval %s: %v", rule.ID, err)
		}
	}

	if err := ss.ApplyRestore(ctx, sessions.RestorePlan{Session: session.Session{ID: "ses_A", Cwd: "/repo"}}); err != nil {
		t.Fatalf("ApplyRestore: %v", err)
	}
	if got, err := ss.todos.List(ctx, "ses_A"); err != nil || len(got) != 0 {
		t.Fatalf("todos after restore = %+v, %v, want cleared", got, err)
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

func testApprovalRule(t *testing.T, scope approval.Scope, scopeKey, toolName string, decision approval.Decision) approval.Rule {
	t.Helper()
	rule, err := approval.NewRule(scope, scopeKey, toolName, "", decision)
	if err != nil {
		t.Fatalf("new approval rule: %v", err)
	}
	return rule
}

func TestApplyRollbackDeletesSubtaskSetAtomically(t *testing.T) {
	ss, _, _ := newWriteSetFixture(t)
	ctx := t.Context()
	parent, err := ss.sessions.Create(ctx, "parent", "/repo")
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	now := time.Now().UTC()
	child, err := ss.sessions.SaveSubtask(ctx, session.Subtask{
		ID: "ses_child", ParentID: parent.ID, StartedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	if err := ss.history.Seed(ctx, child.ID, []chat.Message{chat.NewUserMessage(chat.NewTextPart("preserve on rollback"))}); err != nil {
		t.Fatalf("seed child history: %v", err)
	}
	if err := ss.processes.SaveTree(ctx, bootstrapSnapshotTree(
		"proc_preserve",
		bootstrapWaitingSnapshot("proc_preserve"),
	), bootstrapCheckpoint(parent.ID, accounting.Snapshot{})); err != nil {
		t.Fatalf("seed process snapshot: %v", err)
	}

	err = ss.ApplyRollback(ctx, sessions.RollbackPlan{
		SessionID: parent.ID, KeepMark: -1, ProcessIDs: []string{"proc_preserve"},
		DropSessionIDs: []string{child.ID, ""},
	})
	if err == nil {
		t.Fatal("ApplyRollback unexpectedly accepted an invalid subtask id")
	}
	if _, err := ss.sessions.Get(ctx, child.ID); err != nil {
		t.Fatalf("child delete was not rolled back: %v", err)
	}
	messages, err := ss.history.Read(ctx, child.ID)
	if err != nil || len(messages) != 1 {
		t.Fatalf("child history after rollback = %+v, %v", messages, err)
	}
	if _, _, err := ss.processes.LoadTree(ctx, "proc_preserve"); err != nil {
		t.Fatalf("process snapshot delete was not rolled back: %v", err)
	}
}

func TestApplyRestoreRollsBackOnTranscriptIdentityConflict(t *testing.T) {
	ss, _, _ := newWriteSetFixture(t)
	ctx := t.Context()
	for _, ses := range []session.Session{
		{ID: "ses_A", Title: "source", Cwd: "/source"},
		{ID: "ses_B", Title: "target", Cwd: "/target"},
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
		SessionID: "ses_A", RunID: "run_shared", ID: "item_shared", CreatedAt: now,
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
		Session:  session.Session{ID: "ses_B", Title: "replacement", Cwd: "/replacement"},
		Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("after"))},
		Runs:     []transcript.Run{restoredRun("ses_B", "run_shared", now)},
	})
	if !errors.Is(err, transcript.ErrIdentityConflict) {
		t.Fatalf("ApplyRestore error = %v, want ErrIdentityConflict", err)
	}

	target, err := ss.sessions.Get(ctx, "ses_B")
	if err != nil || target.Title != "target" || target.Cwd != "/target" {
		t.Fatalf("target session after rollback = %+v, %v", target, err)
	}
	messages, err := ss.history.Read(ctx, "ses_B")
	if err != nil || len(messages) != 1 {
		t.Fatalf("target history after rollback = %+v, %v", messages, err)
	}
	targetRuns, err := ss.runs.ListRuns(ctx, "ses_B")
	if err != nil || len(targetRuns) != 1 || targetRuns[0].ID != "run_target" {
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

	if err := ss.ApplyDelete(ctx, sessions.DeletePlan{SessionIDs: []string{"ses_goal"}}); err != nil {
		t.Fatalf("ApplyDelete: %v", err)
	}
	if _, ok, err := ss.goals.Get(ctx, "ses_goal"); err != nil || ok {
		t.Fatalf("goal survived the delete cascade: ok=%v err=%v", ok, err)
	}
}
