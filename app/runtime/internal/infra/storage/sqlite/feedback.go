package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/feedback"
)

// FeedbackStore persists the append-only quality ledger. The DB must have been
// opened via Open so the feedback_entries table exists.
type FeedbackStore struct {
	db *sql.DB
}

// NewFeedbackStore wires db to the feedback receiver.
func NewFeedbackStore(db *sql.DB) *FeedbackStore {
	return &FeedbackStore{db: db}
}

// Append stores a validated immutable feedback entry. It joins any ambient
// lifecycle write set through conn, even though feedback is normally an
// independent user action.
func (s *FeedbackStore) Append(ctx context.Context, entry feedback.Entry) error {
	if err := entry.Validate(); err != nil {
		return err
	}
	if _, err := conn(ctx, s.db).ExecContext(ctx,
		`INSERT INTO feedback_entries(session_id, run_id, item_id, rating, text, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		entry.SessionID, entry.RunID, entry.ItemID, entry.Rating, entry.Text, entry.CreatedAt.UnixMilli()); err != nil {
		return fmt.Errorf("sqlite: append feedback: %w", err)
	}
	return nil
}
