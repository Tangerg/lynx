package persistence_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/lynx/app/runtime/internal/application/conversations"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/sqlite"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/runfixture"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/sessionfixture"
	"github.com/Tangerg/lynx/core/chat"
)

func newCompactionFixture(t *testing.T) (*sql.DB, *sqlite.MessageStore, *sqlite.RunStore, *conversations.Messages) {
	t.Helper()
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	messages := sqlite.NewMessageStore(db)
	runs := sqlite.NewRunStore(db)
	compactions := persistence.NewConversationCompactions(
		messages,
		runs,
		func(ctx context.Context, fn func(context.Context) error) error {
			return sqlite.RunInTx(ctx, db, fn)
		},
	)
	service := conversations.NewMessages(messages, compactions)
	ses := sessionfixture.MustRestore(session.Snapshot{ID: "ses_long", Title: "long", Workspace: sessionfixture.MustWorkspace("/work")})
	if err := sqlite.NewSessionStore(db).Insert(t.Context(), ses); err != nil {
		t.Fatal(err)
	}
	return db, messages, runs, service
}

func seedCompactionHistory(t *testing.T, messages *sqlite.MessageStore, runs *sqlite.RunStore) []chat.Message {
	t.Helper()
	history := make([]chat.Message, 0, 8)
	for index := range 4 {
		history = append(history,
			chat.NewUserMessage(chat.NewTextPart("question")),
			chat.NewAssistantMessage(chat.NewTextPart("answer")),
		)
		mark := len(history)
		at := time.Unix(int64(index+1), 0).UTC()
		terminal := runfixture.MustRestore(run.Snapshot{
			ID: "run_" + string(rune('a'+index)), SessionID: "ses_long",
			State: run.Completed, CreatedAt: at, FinishedAt: at, UpdatedAt: at,
			MessageMark: mark,
		})
		if err := runs.Restore(t.Context(), terminal); err != nil {
			t.Fatal(err)
		}
	}
	if err := messages.Write(t.Context(), "ses_long", history...); err != nil {
		t.Fatal(err)
	}
	return history
}

func compactedMessages() []chat.Message {
	return []chat.Message{
		chat.NewSystemMessage("summary"),
		chat.NewUserMessage(chat.NewTextPart("question")),
		chat.NewAssistantMessage(chat.NewTextPart("answer")),
	}
}

func TestConversationCompactionRebasesRunsAcrossRepeatedLongTurns(t *testing.T) {
	db, messages, runs, service := newCompactionFixture(t)
	seedCompactionHistory(t, messages, runs)
	if err := service.RewriteForCompaction(t.Context(), "ses_long", 8, 6, 1, compactedMessages()...); err != nil {
		t.Fatal(err)
	}
	after, err := messages.Read(t.Context(), "ses_long")
	if err != nil {
		t.Fatal(err)
	}
	afterRuns, err := runs.ListRuns(t.Context(), "ses_long")
	if err != nil {
		t.Fatal(err)
	}
	wantMarks := []int{1, 1, 1, 3}
	for index, current := range afterRuns {
		if current.MessageMark() != wantMarks[index] {
			t.Errorf("Run %s mark = %d, want %d", current.ID(), current.MessageMark(), wantMarks[index])
		}
	}
	if _, resolveForkBoundaryErr := sessions.ResolveForkBoundary(after, afterRuns, "run_b"); resolveForkBoundaryErr != nil {
		t.Fatalf("compacted Run timeline must remain forkable: %v", resolveForkBoundaryErr)
	}

	// Continue the same single-client conversation until it compacts again. Old
	// boundaries are already in the first replacement's coordinates; the second
	// rewrite must transform them once, not reuse their original counts.
	for range 5 {
		if writeErr := messages.Write(t.Context(), "ses_long", chat.NewUserMessage(chat.NewTextPart("next turn"))); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	at := time.Unix(10, 0).UTC()
	latest := runfixture.MustRestore(run.Snapshot{
		ID: "run_e", SessionID: "ses_long", State: run.Completed,
		CreatedAt: at, FinishedAt: at, UpdatedAt: at, MessageMark: 8,
	})
	if restoreErr := runs.Restore(t.Context(), latest); restoreErr != nil {
		t.Fatal(restoreErr)
	}
	if rewriteForCompactionErr := service.RewriteForCompaction(t.Context(), "ses_long", 8, 6, 1, compactedMessages()...); rewriteForCompactionErr != nil {
		t.Fatal(rewriteForCompactionErr)
	}
	after, err = messages.Read(t.Context(), "ses_long")
	if err != nil {
		t.Fatal(err)
	}
	afterRuns, err = runs.ListRuns(t.Context(), "ses_long")
	if err != nil {
		t.Fatal(err)
	}
	for _, current := range afterRuns {
		want := 1
		if current.ID() == "run_e" {
			want = 3
		}
		if current.MessageMark() != want {
			t.Errorf("Run %s mark after second compaction = %d, want %d", current.ID(), current.MessageMark(), want)
		}
		if current.MessageMark() > len(after) {
			t.Errorf("Run %s mark %d exceeds current history %d", current.ID(), current.MessageMark(), len(after))
		}
	}
	if _, err := sessions.ResolveForkBoundary(after, afterRuns, "run_e"); err != nil {
		t.Fatalf("twice-compacted Run timeline must remain forkable: %v", err)
	}
	var invalid int
	if err := db.QueryRowContext(t.Context(), `
		SELECT COUNT(*) FROM runs
		 WHERE session_id = ? AND state = 'terminal'
		   AND (message_mark < 0 OR message_mark > (
		       SELECT COUNT(*) FROM messages WHERE conversation_id = ?
		   ))`, "ses_long", "ses_long").Scan(&invalid); err != nil {
		t.Fatal(err)
	}
	if invalid != 0 {
		t.Fatalf("SQLite contains %d terminal Runs outside the current conversation coordinates", invalid)
	}
}

func TestConversationCompactionRollsBackHistoryWhenRunRebaseFails(t *testing.T) {
	db, messages, runs, service := newCompactionFixture(t)
	before := seedCompactionHistory(t, messages, runs)
	if _, err := db.ExecContext(t.Context(), `CREATE TRIGGER reject_compaction_mark
		BEFORE UPDATE OF message_mark ON runs
		BEGIN SELECT RAISE(ABORT, 'injected Run watermark failure'); END`); err != nil {
		t.Fatal(err)
	}

	if err := service.RewriteForCompaction(t.Context(), "ses_long", 8, 6, 1, compactedMessages()...); err == nil {
		t.Fatal("injected Run-watermark failure must fail the whole compaction")
	}
	after, err := messages.Read(t.Context(), "ses_long")
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("history length after rollback = %d, want %d", len(after), len(before))
	}
	afterRuns, err := runs.ListRuns(t.Context(), "ses_long")
	if err != nil {
		t.Fatal(err)
	}
	for index, current := range afterRuns {
		want := (index + 1) * 2
		if current.MessageMark() != want {
			t.Errorf("Run %s mark after rollback = %d, want %d", current.ID(), current.MessageMark(), want)
		}
	}
}
