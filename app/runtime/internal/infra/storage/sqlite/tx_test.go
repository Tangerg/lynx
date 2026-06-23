package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

// TestRunInTx_AtomicAcrossStores is the guarantee sessions.import / rollback
// rely on: a write-set spanning several stores (session row + chat messages,
// which live in different tables behind different store types) commits or rolls
// back as one. It also proves the conn(ctx) threading works — the two stores'
// writes join the SAME transaction rather than each opening its own (which
// would deadlock under MaxOpenConns(1)).
func TestRunInTx_AtomicAcrossStores(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	sess := sqlite.NewSessionService(db)
	msg := sqlite.NewMessageStore(db)
	ctx := context.Background()

	// A multi-store write-set that fails mid-way must leave NO partial state.
	boom := errors.New("boom")
	err = sqlite.RunInTx(ctx, db, func(ctx context.Context) error {
		if err := sess.Restore(ctx, session.Session{ID: "s1", Title: "t"}); err != nil {
			return err
		}
		if err := msg.Write(ctx, "s1", chat.NewUserMessage("hi")); err != nil {
			return err
		}
		return boom // a later step fails (e.g. a DB IO error during import)
	})
	if !errors.Is(err, boom) {
		t.Fatalf("RunInTx err = %v, want boom", err)
	}
	if _, gerr := sess.Get(ctx, "s1"); !errors.Is(gerr, session.ErrNotFound) {
		t.Errorf("session survived a rolled-back tx (Get err = %v, want ErrNotFound)", gerr)
	}
	if n, _ := msg.Count(ctx, "s1"); n != 0 {
		t.Errorf("messages survived a rolled-back tx: count = %d, want 0", n)
	}

	// A successful write-set commits both stores.
	if err := sqlite.RunInTx(ctx, db, func(ctx context.Context) error {
		if err := sess.Restore(ctx, session.Session{ID: "s2", Title: "t"}); err != nil {
			return err
		}
		return msg.Write(ctx, "s2", chat.NewUserMessage("hi"))
	}); err != nil {
		t.Fatalf("commit tx: %v", err)
	}
	if _, gerr := sess.Get(ctx, "s2"); gerr != nil {
		t.Errorf("committed session missing: %v", gerr)
	}
	if n, _ := msg.Count(ctx, "s2"); n != 1 {
		t.Errorf("committed messages = %d, want 1", n)
	}
}
