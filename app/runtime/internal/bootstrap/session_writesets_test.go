package bootstrap

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/conversation"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
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
	ss := sessionStores{
		sessions:   sqlite.NewSessionStore(db),
		transcript: sqlite.NewTranscriptStore(db),
		interrupts: ints,
		runs:       runs,
		history:    conversation.NewMessages(sqlite.NewMessageStore(db)),
		forgetter:  noopForgetter{},
		tx: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlite.RunInTx(ctx, db, fn)
		},
	}
	return ss, runs, ints
}

func park(t *testing.T, runs *sqlite.RunStateStore, ints *sqlite.InterruptStore, sessionID, runID string) {
	t.Helper()
	ctx := context.Background()
	if err := runs.Admit(ctx, execution.RunDraft{RunID: runID, SessionID: sessionID, CreatedAt: time.Unix(0, 0)}); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := runs.Suspend(ctx, sessionID, runID); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if err := ints.Put(ctx, interrupts.Pending{RunID: runID, SessionID: sessionID, CreatedAt: time.Unix(0, 0)}); err != nil {
		t.Fatalf("put interrupt: %v", err)
	}
}

// TestApplyCancelDropsInterruptAndTerminalizes: abandoning a parked run frees both
// the resumable record and the durable admission slot, atomically.
func TestApplyCancelDropsInterruptAndTerminalizes(t *testing.T) {
	ss, runs, ints := newWriteSetFixture(t)
	ctx := context.Background()
	park(t, runs, ints, "ses_A", "run_1")

	if err := ss.ApplyCancel(ctx, "ses_A", "run_1"); err != nil {
		t.Fatalf("ApplyCancel: %v", err)
	}
	if open, _ := ints.List(ctx, "ses_A"); len(open) != 0 {
		t.Fatalf("interrupt survived cancel: %+v", open)
	}
	// The admission row is terminal, so the session can start a fresh run.
	if err := runs.Admit(ctx, execution.RunDraft{RunID: "run_2", SessionID: "ses_A"}); err != nil {
		t.Fatalf("admit after cancel = %v, want the slot freed", err)
	}
}

// TestApplyRollbackDropsRunsAndTerminalizes: a rollback that abandons a parked run
// drops its interrupt and terminalizes the admission slot when Terminate is set.
func TestApplyRollbackDropsRunsAndTerminalizes(t *testing.T) {
	ss, runs, ints := newWriteSetFixture(t)
	ctx := context.Background()
	park(t, runs, ints, "ses_A", "run_1")

	if err := ss.ApplyRollback(ctx, sessions.RollbackPlan{
		SessionID:  "ses_A",
		RunID:      "run_1",
		KeepMark:   -1,
		DropRunIDs: []string{"run_1"},
		Terminate:  true,
	}); err != nil {
		t.Fatalf("ApplyRollback: %v", err)
	}
	if open, _ := ints.List(ctx, "ses_A"); len(open) != 0 {
		t.Fatalf("dropped run's interrupt survived rollback: %+v", open)
	}
	if err := runs.Admit(ctx, execution.RunDraft{RunID: "run_2", SessionID: "ses_A"}); err != nil {
		t.Fatalf("admit after rollback = %v, want the slot freed", err)
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
	park(t, runs, ints, "ses_A", "run_1")

	if err := ss.ApplyDelete(ctx, "ses_A"); err != nil {
		t.Fatalf("ApplyDelete: %v", err)
	}
	if open, _ := ints.List(ctx, "ses_A"); len(open) != 0 {
		t.Fatalf("interrupt survived delete: %+v", open)
	}
	if _, err := ss.sessions.Get(ctx, "ses_A"); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("session survived delete: %v", err)
	}
	// The non-terminal admission row is gone (not just terminal), so a fresh admit
	// succeeds — proving the delete cascade dropped the runs rows.
	if err := runs.Admit(ctx, execution.RunDraft{RunID: "run_2", SessionID: "ses_A"}); err != nil {
		t.Fatalf("admit after delete = %v, want the slot freed", err)
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
