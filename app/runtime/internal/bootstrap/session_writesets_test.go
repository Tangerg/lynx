package bootstrap

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/conversation"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
	sqlite "github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

type noopForgetter struct{}

func (noopForgetter) ForgetSession(string) {}

// newWriteSetFixture builds the composition-root sessionStores adapter over a
// fresh sqlite database so the atomic write-sets run against the real stores +
// transactor.
func newWriteSetFixture(t *testing.T) (sessionStores, *sqlite.RunStateStore, *sqlite.InterruptStore) {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	runs := sqlite.NewRunStateStore(db)
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
		forgetter:  noopForgetter{},
		tx: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlite.RunInTx(ctx, db, fn)
		},
	}
	return ss, runs, ints
}

func park(t *testing.T, runs *sqlite.RunStateStore, ints *sqlite.InterruptStore, processes *sqlite.ProcessStore, sessionID, runID string) string {
	t.Helper()
	ctx := context.Background()
	processID := "proc_" + runID
	if err := processes.Save(ctx, core.ProcessSnapshot{ID: processID, AgentName: "chat", Status: core.StatusWaiting}); err != nil {
		t.Fatalf("save process snapshot: %v", err)
	}
	if err := runs.Admit(ctx, execution.RunDraft{RunID: runID, SessionID: sessionID, CreatedAt: time.Unix(0, 0)}); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := runs.Suspend(ctx, sessionID, runID); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if err := ints.Put(ctx, interrupts.Pending{RunID: runID, SessionID: sessionID, ProcessID: processID, CreatedAt: time.Unix(0, 0)}); err != nil {
		t.Fatalf("put interrupt: %v", err)
	}
	return processID
}

// TestApplyCancelDropsInterruptAndTerminalizes: abandoning a parked run frees both
// the resumable record and the durable admission slot, atomically.
func TestApplyCancelDropsInterruptAndTerminalizes(t *testing.T) {
	ss, runs, ints := newWriteSetFixture(t)
	ctx := t.Context()
	processID := park(t, runs, ints, ss.processes, "ses_A", "run_1")
	outcome := execution.OutcomeCanceled
	finishedAt := time.Date(2026, 7, 13, 2, 3, 4, 0, time.UTC)

	if err := ss.ApplyCancel(ctx, sessions.CancelPlan{ProcessID: processID, Run: transcript.Run{
		SessionID: "ses_A", ID: "run_1", State: execution.Canceled,
		Outcome: &outcome, Result: &transcript.RunResult{},
		FinishedAt: finishedAt, UpdatedAt: finishedAt, MessageMark: 0,
	}}); err != nil {
		t.Fatalf("ApplyCancel: %v", err)
	}
	if open, _ := ints.List(ctx, "ses_A"); len(open) != 0 {
		t.Fatalf("interrupt survived cancel: %+v", open)
	}
	if _, err := ss.processes.Load(ctx, processID); !errors.Is(err, core.ErrSnapshotNotFound) {
		t.Fatalf("process snapshot after cancel = %v, want not found", err)
	}
	// The admission row is terminal, so the session can start a fresh run.
	if err := runs.Admit(ctx, execution.RunDraft{RunID: "run_2", SessionID: "ses_A"}); err != nil {
		t.Fatalf("admit after cancel = %v, want the slot freed", err)
	}
	_, transcriptRuns, err := ss.transcript.List(ctx, "ses_A")
	if err != nil || len(transcriptRuns) != 1 || transcriptRuns[0].State != execution.Canceled {
		t.Fatalf("terminal transcript = %+v (err %v), want canceled run", transcriptRuns, err)
	}
}

// TestApplyRollbackDropsRunsAndTerminalizes: a rollback that abandons a parked run
// drops its interrupt and terminalizes the admission slot when Terminate is set.
func TestApplyRollbackDropsRunsAndTerminalizes(t *testing.T) {
	ss, runs, ints := newWriteSetFixture(t)
	ctx := context.Background()
	processID := park(t, runs, ints, ss.processes, "ses_A", "run_1")
	if err := ss.todos.Replace(ctx, "ses_A", []todo.Item{{Content: "future work", Status: todo.StatusPending}}); err != nil {
		t.Fatalf("seed todos: %v", err)
	}

	if err := ss.ApplyRollback(ctx, sessions.RollbackPlan{
		SessionID:  "ses_A",
		RunID:      "run_1",
		KeepMark:   -1,
		DropRunIDs: []string{"run_1"},
		ProcessIDs: []string{processID},
		Terminate:  true,
	}); err != nil {
		t.Fatalf("ApplyRollback: %v", err)
	}
	if open, _ := ints.List(ctx, "ses_A"); len(open) != 0 {
		t.Fatalf("dropped run's interrupt survived rollback: %+v", open)
	}
	if _, err := ss.processes.Load(ctx, processID); !errors.Is(err, core.ErrSnapshotNotFound) {
		t.Fatalf("process snapshot after rollback = %v, want not found", err)
	}
	if err := runs.Admit(ctx, execution.RunDraft{RunID: "run_2", SessionID: "ses_A"}); err != nil {
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
		Messages: []chat.Message{chat.NewUserMessage("hello")},
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
func TestApplyDeleteRemovesRunRows(t *testing.T) {
	ss, runs, ints := newWriteSetFixture(t)
	ctx := context.Background()
	if err := ss.sessions.Restore(ctx, session.Session{ID: "ses_A"}); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	processID := park(t, runs, ints, ss.processes, "ses_A", "run_1")
	if err := ss.todos.Replace(ctx, "ses_A", []todo.Item{{Content: "owned", Status: todo.StatusPending}}); err != nil {
		t.Fatalf("seed todos: %v", err)
	}
	if err := ss.approvals.Put(ctx, approval.Rule{ID: "session", Scope: approval.ScopeSession, ScopeKey: "ses_A", Tool: "shell", Decision: approval.Allow}); err != nil {
		t.Fatalf("seed approval: %v", err)
	}

	if err := ss.ApplyDelete(ctx, sessions.DeletePlan{SessionIDs: []string{"ses_A"}}); err != nil {
		t.Fatalf("ApplyDelete: %v", err)
	}
	if open, _ := ints.List(ctx, "ses_A"); len(open) != 0 {
		t.Fatalf("interrupt survived delete: %+v", open)
	}
	if _, err := ss.processes.Load(ctx, processID); !errors.Is(err, core.ErrSnapshotNotFound) {
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
	if err := runs.Admit(ctx, execution.RunDraft{RunID: "run_2", SessionID: "ses_A"}); err != nil {
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
	for _, rule := range []approval.Rule{
		{ID: "session", Scope: approval.ScopeSession, ScopeKey: "ses_A", Tool: "shell", Decision: approval.Allow},
		{ID: "project", Scope: approval.ScopeProject, ScopeKey: "/repo", Tool: "write", Decision: approval.Allow},
		{ID: "global", Scope: approval.ScopeGlobal, Tool: "read", Decision: approval.Allow},
	} {
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
	rules, err := ss.approvals.Visible(ctx, "ses_A", "/repo")
	if err != nil {
		t.Fatalf("visible approvals: %v", err)
	}
	ids := make(map[string]bool, len(rules))
	for _, rule := range rules {
		ids[rule.ID] = true
	}
	if ids["session"] || !ids["project"] || !ids["global"] || len(ids) != 2 {
		t.Fatalf("approvals after restore = %v, want project+global only", ids)
	}
}

func TestApplyRollbackDeletesSubtaskSetAtomically(t *testing.T) {
	ss, _, _ := newWriteSetFixture(t)
	ctx := t.Context()
	parent, err := ss.sessions.Create(ctx, "parent", "/repo")
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	child, err := ss.sessions.CreateSubtask(ctx, "ses_child", parent.ID)
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	if err := ss.history.Seed(ctx, child.ID, []chat.Message{chat.NewUserMessage("preserve on rollback")}); err != nil {
		t.Fatalf("seed child history: %v", err)
	}
	if err := ss.processes.Save(ctx, core.ProcessSnapshot{ID: "proc_preserve", AgentName: "chat", Status: core.StatusWaiting}); err != nil {
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
	if _, err := ss.processes.Load(ctx, "proc_preserve"); err != nil {
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
	if err := ss.transcript.PutRun(ctx, transcript.Run{SessionID: "ses_A", ID: "run_shared", UpdatedAt: now}); err != nil {
		t.Fatalf("seed source run: %v", err)
	}
	if err := ss.transcript.AppendItem(ctx, transcript.Item{
		SessionID: "ses_A", RunID: "run_shared", ID: "item_shared", CreatedAt: now,
	}); err != nil {
		t.Fatalf("seed source item: %v", err)
	}
	if err := ss.transcript.PutRun(ctx, transcript.Run{SessionID: "ses_B", ID: "run_target", UpdatedAt: now}); err != nil {
		t.Fatalf("seed target run: %v", err)
	}
	if err := ss.history.Seed(ctx, "ses_B", []chat.Message{chat.NewUserMessage("before")}); err != nil {
		t.Fatalf("seed target history: %v", err)
	}

	err := ss.ApplyRestore(ctx, sessions.RestorePlan{
		Session:  session.Session{ID: "ses_B", Title: "replacement", Cwd: "/replacement"},
		Messages: []chat.Message{chat.NewUserMessage("after")},
		Runs:     []transcript.Run{{SessionID: "ses_B", ID: "run_shared", UpdatedAt: now}},
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
	_, targetRuns, err := ss.transcript.List(ctx, "ses_B")
	if err != nil || len(targetRuns) != 1 || targetRuns[0].ID != "run_target" {
		t.Fatalf("target transcript after rollback = %+v, %v", targetRuns, err)
	}
	sourceItems, sourceRuns, err := ss.transcript.List(ctx, "ses_A")
	if err != nil || len(sourceItems) != 1 || len(sourceRuns) != 1 {
		t.Fatalf("source transcript after conflict = items=%+v runs=%+v err=%v", sourceItems, sourceRuns, err)
	}
}
