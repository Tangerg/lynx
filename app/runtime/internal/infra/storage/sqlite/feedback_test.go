package sqlite_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/feedback"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

func TestFeedbackStoreAppendsEntry(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewFeedbackStore(db)
	entry, err := feedback.NewEntry("ses_1", "run_1", "item_1", feedback.RatingNegative, "wrong answer", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Append(t.Context(), entry); err != nil {
		t.Fatalf("Append: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM feedback_entries WHERE item_id = ? AND rating = ?`, "item_1", feedback.RatingNegative).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
}
