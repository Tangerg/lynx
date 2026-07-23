// Package feedback contains the feedback.create use case. It turns a delivery
// request into one validated, durable quality observation; it deliberately has
// no read model because the public protocol promises write-only collection.
package feedback

import (
	"context"
	"time"

	feedbackdomain "github.com/Tangerg/lynx/app/runtime/internal/domain/feedback"
)

// Command is the application input for recording one quality signal.
type Command struct {
	SessionID string
	RunID     string
	ItemID    string
	Rating    feedbackdomain.Rating
	Text      string
}

// Recorder owns the feedback write use case.
type Recorder struct {
	store feedbackdomain.Store
}

// New wires the real durable receiver for feedback records.
func New(store feedbackdomain.Store) *Recorder {
	return &Recorder{store: store}
}

// Record validates and appends one immutable feedback observation.
func (r *Recorder) Record(ctx context.Context, command Command) error {
	entry, err := feedbackdomain.NewEntry(
		command.SessionID,
		command.RunID,
		command.ItemID,
		command.Rating,
		command.Text,
		time.Now().UTC(),
	)
	if err != nil {
		return err
	}
	return r.store.Append(ctx, entry)
}
