package runtimeembedded

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/feedback"
)

type feedbackBinding interface {
	CreateFeedback(context.Context, protocol.FeedbackRequest, embedded.CommandOptions) error
}

type feedbackAdapter struct{ runtime *Runtime }

var _ feedback.Service = (*feedbackAdapter)(nil)

func (adapter *feedbackAdapter) Record(ctx context.Context, signal feedback.Signal) error {
	r := adapter.runtime
	if err := signal.Validate(); err != nil {
		return err
	}
	options, err := r.commandOptions()
	if err != nil {
		return err
	}
	return classifyError(r.feedback.CreateFeedback(ctx, protocol.FeedbackRequest{
		SessionID: signal.SessionID, RunID: signal.RunID, ItemID: signal.ItemID,
		Rating: protocol.FeedbackRating(signal.Rating), Text: signal.Text,
	}, options))
}
