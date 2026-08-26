package runsegment

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/sqlite"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/itemfixture"
)

func TestChildOpeningAtomicallyCommitsRunAndParentSpawningItem(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "runtime.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	runStore := sqlite.NewRunStore(db)
	transcriptStore := sqlite.NewTranscriptStore(db)
	root := run.Draft{
		RunID: "run_root", SessionID: "session_1", SegmentID: "segment_root",
		CreatedAt: time.Unix(1, 0),
	}
	if err := runStore.Admit(t.Context(), root); err != nil {
		t.Fatalf("admit root: %v", err)
	}
	effects := mustNewEffects(Config{
		State:      runStore,
		Transcript: transcriptStore,
		Tx: func(ctx context.Context, apply func(context.Context) error) error {
			return sqlite.RunInTx(ctx, db, apply)
		},
	})
	arguments, err := tool.ParseArguments(`{"description":"delegate"}`)
	if err != nil {
		t.Fatalf("parse arguments: %v", err)
	}
	spawningItem := itemfixture.MustRestore(itemfixture.Input{
		SessionID:  "session_1",
		RunID:      "run_root",
		ID:         "item_delegate",
		Status:     transcript.ItemRunning,
		Kind:       transcript.ToolCall,
		OccurredAt: time.Unix(2, 0),
		Tool: &transcript.ToolInvocation{
			Name:      "delegate_task",
			Arguments: arguments,
		},
	})
	child := run.Draft{
		RunID: "run_child", SessionID: "session_1", SegmentID: "segment_child",
		SpawnedByItemID: spawningItem.ID(), ParentRunID: root.RunID, RootRunID: root.RunID,
		CreatedAt: time.Unix(3, 0),
	}
	if err := effects.CommitOpening(t.Context(), runs.OpeningCommit{
		CommitID: "run_commit_child_opening", Admit: &child,
		Events: []runs.EventCommit{{
			RunID: root.RunID, SessionID: root.SessionID, SegmentID: root.SegmentID,
			Items: []transcript.Item{spawningItem},
		}},
	}); err != nil {
		t.Fatalf("CommitOpening: %v", err)
	}

	persistedChild, found, err := runStore.Run(t.Context(), child.RunID)
	if err != nil || !found {
		t.Fatalf("read child: found=%v error=%v", found, err)
	}
	if persistedChild.Lineage().SpawnedByItemID != spawningItem.ID() ||
		persistedChild.Lineage().ParentRunID != root.RunID ||
		persistedChild.Lineage().RootRunID != root.RunID {
		t.Fatalf("persisted child = %+v, want complete lineage", persistedChild)
	}
	items, err := transcriptStore.List(t.Context(), root.SessionID)
	if err != nil {
		t.Fatalf("list transcript: %v", err)
	}
	if len(items) != 1 || items[0].ID() != spawningItem.ID() || items[0].Status() != transcript.ItemRunning {
		t.Fatalf("persisted parent items = %+v, want running spawning item", items)
	}

	rollbackErr := errors.New("reject parent item projection")
	failingEffects := mustNewEffects(Config{
		State: runStore,
		Transcript: appendThenFail{
			store: transcriptStore,
			err:   rollbackErr,
		},
		Tx: func(ctx context.Context, apply func(context.Context) error) error {
			return sqlite.RunInTx(ctx, db, apply)
		},
	})
	rolledBackItem := itemfixture.MustRestore(itemfixture.Input{
		SessionID: "session_1", RunID: "run_root", ID: "item_rollback",
		Status: transcript.ItemRunning, Kind: transcript.ToolCall, OccurredAt: time.Unix(4, 0),
		Tool: &transcript.ToolInvocation{Name: "delegate_task", Arguments: arguments},
	})
	rolledBackChild := child
	rolledBackChild.RunID = "run_rollback"
	rolledBackChild.SegmentID = "segment_rollback"
	rolledBackChild.SpawnedByItemID = rolledBackItem.ID()
	rolledBackChild.CreatedAt = time.Unix(5, 0)
	err = failingEffects.CommitOpening(t.Context(), runs.OpeningCommit{
		CommitID: "run_commit_child_failure", Admit: &rolledBackChild,
		Events: []runs.EventCommit{{
			RunID: root.RunID, SessionID: root.SessionID, SegmentID: root.SegmentID,
			Items: []transcript.Item{rolledBackItem},
		}},
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("rolled-back CommitOpening error = %v, want %v", err, rollbackErr)
	}
	if _, found, err := runStore.Run(t.Context(), rolledBackChild.RunID); err != nil || found {
		t.Fatalf("rolled-back child: found=%v error=%v, want absent", found, err)
	}
	items, err = transcriptStore.List(t.Context(), root.SessionID)
	if err != nil {
		t.Fatalf("list transcript after rollback: %v", err)
	}
	if len(items) != 1 || items[0].ID() != spawningItem.ID() {
		t.Fatalf("items after rollback = %+v, want only committed spawning item", items)
	}
}

func TestStartedChildOpeningReconcilesOnlyItsExactWriteSet(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "runtime.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	runStore := sqlite.NewRunStore(db)
	transcriptStore := sqlite.NewTranscriptStore(db)
	childStarts := sqlite.NewChildRunStartReservationStore(db)
	root := run.Draft{
		RunID: "run_root", SessionID: "session_1", SegmentID: "segment_root",
		CreatedAt: time.Unix(1, 0).UTC(),
	}
	if err := runStore.Admit(ctx, root); err != nil {
		t.Fatalf("admit root: %v", err)
	}
	arguments, err := tool.ParseArguments(`{"description":"delegate"}`)
	if err != nil {
		t.Fatalf("parse arguments: %v", err)
	}
	spawningItem := itemfixture.MustRestore(itemfixture.Input{
		SessionID: root.SessionID, RunID: root.RunID, ID: "item_delegate",
		Status: transcript.ItemRunning, Kind: transcript.ToolCall, OccurredAt: time.Unix(2, 0).UTC(),
		Tool: &transcript.ToolInvocation{Name: "delegate_task", Arguments: arguments},
	})
	startedAt := time.Unix(3, 0).UTC()
	child := run.Draft{
		RunID: "run_child", SessionID: root.SessionID, SegmentID: "segment_child",
		SpawnedByItemID: spawningItem.ID(), ParentRunID: root.RunID, RootRunID: root.RunID,
		CreatedAt: startedAt,
	}
	reservation := runs.ChildRunStartReservation{
		SessionID: root.SessionID, ExecutorID: "executor_1",
		Member: runs.ExecutorMember{
			MemberID: "member_child", ParentID: "member_root", SpawnCallID: "call_delegate",
		},
		Binding: runs.ChildRunBinding{
			MemberID: "member_child", RunID: child.RunID, ParentRunID: root.RunID,
		},
		SegmentID: child.SegmentID, SpawnedByItemID: spawningItem.ID(),
		RootRunID: root.RunID, StartedAt: startedAt,
	}
	loseReceipt := false
	commitCtx, cancelCommit := context.WithCancel(ctx)
	t.Cleanup(cancelCommit)
	effects := mustNewEffects(Config{
		State: runStore, Transcript: transcriptStore, ChildRunStarts: childStarts,
		Tx: func(ctx context.Context, apply func(context.Context) error) error {
			err := sqlite.RunInTx(ctx, db, apply)
			if err == nil && loseReceipt {
				loseReceipt = false
				cancelCommit()
				return errors.New("lost child opening commit receipt")
			}
			return err
		},
	})
	if err := effects.ReserveChildRunStart(ctx, reservation); err != nil {
		t.Fatalf("ReserveChildRunStart: %v", err)
	}
	opening := runs.OpeningCommit{
		CommitID: "run_commit_child_started", Admit: &child,
		Events: []runs.EventCommit{{
			RunID: root.RunID, SessionID: root.SessionID, SegmentID: root.SegmentID,
			Items: []transcript.Item{spawningItem},
		}},
	}
	loseReceipt = true
	if err := effects.CommitStartedChildRun(commitCtx, reservation, opening); err != nil {
		t.Fatalf("ambiguous CommitStartedChildRun = %v, want reconciled success", err)
	}
	matched, err := runStore.RunCommitCommitted(
		ctx, child.SessionID, child.RunID, child.SegmentID, opening.CommitID,
	)
	if err != nil || !matched {
		t.Fatalf("child opening marker matched=%t err=%v, want true/nil", matched, err)
	}
	if err := effects.CommitStartedChildRun(ctx, reservation, opening); err != nil {
		t.Fatalf("exact concluded child opening = %v, want idempotent success", err)
	}
	opening.CommitID = "run_commit_other_child_started"
	if err := effects.CommitStartedChildRun(ctx, reservation, opening); err == nil {
		t.Fatal("different child opening write-set reused a concluded reservation")
	}
	items, err := transcriptStore.List(ctx, root.SessionID)
	if err != nil || len(items) != 1 || items[0].ID() != spawningItem.ID() {
		t.Fatalf("child opening items = %#v err=%v, want one spawning Item", items, err)
	}
	requireChildOpeningSQLiteHealthy(t, ctx, db)
}

type appendThenFail struct {
	store *sqlite.TranscriptStore
	err   error
}

func (a appendThenFail) AppendItem(ctx context.Context, item transcript.Item) error {
	if err := a.store.AppendItem(ctx, item); err != nil {
		return err
	}
	return a.err
}

func requireChildOpeningSQLiteHealthy(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	var integrity string
	if err := db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("integrity_check = %q err=%v, want ok", integrity, err)
	}
	foreignKeys, err := db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("foreign_key_check: %v", err)
	}
	defer foreignKeys.Close()
	if foreignKeys.Next() {
		t.Fatal("foreign_key_check reported a violation")
	}
	if err := foreignKeys.Err(); err != nil {
		t.Fatalf("foreign_key_check rows: %v", err)
	}
}
