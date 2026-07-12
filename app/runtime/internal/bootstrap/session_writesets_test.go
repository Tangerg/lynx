package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/conversation"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
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
	if err := runs.Suspend(ctx, sessionID); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if err := ints.Put(ctx, interrupts.Pending{RunID: runID, SessionID: sessionID, Interrupts: json.RawMessage("[]"), CreatedAt: time.Unix(0, 0)}); err != nil {
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

	if err := ss.ApplyRollback(ctx, execution.RollbackPlan{
		SessionID:  "ses_A",
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

	child, err := ss.ApplyFork(ctx, execution.ForkPlan{
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
