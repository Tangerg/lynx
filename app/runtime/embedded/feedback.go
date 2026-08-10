package embedded

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// CreateFeedback records one quality signal.
func (r *Runtime) CreateFeedback(ctx context.Context, request protocol.FeedbackRequest, options CommandOptions) error {
	return invokeAck(ctx, r, "feedback.create", request, commandOptions(options))
}
