package server

import (
	"context"
	"errors"
	"fmt"

	feedbackapp "github.com/Tangerg/scope/app/runtime/internal/application/feedback"
	feedbackdomain "github.com/Tangerg/scope/app/runtime/internal/domain/feedback"
	"github.com/Tangerg/scope/app/runtime/protocol"
)

// CreateFeedback records an ungated quality signal in the runtime's durable
// feedback ledger. The write-only protocol shape intentionally has no readback,
// but a successful ack always means the application receiver accepted it.
func (s *Server) CreateFeedback(ctx context.Context, in protocol.FeedbackRequest) error {
	err := s.feedback.Record(ctx, feedbackapp.Command{
		SessionID: in.SessionID,
		RunID:     in.RunID,
		ItemID:    in.ItemID,
		Rating:    feedbackdomain.Rating(in.Rating),
		Text:      in.Text,
	})
	if errors.Is(err, feedbackdomain.ErrInvalid) {
		return fmt.Errorf("%w: %v", protocol.ErrInvalidParams, err)
	}
	return err
}
