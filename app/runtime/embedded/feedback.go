package embedded

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// CreateFeedback records one quality signal.
func (r *Runtime) CreateFeedback(ctx context.Context, request protocol.FeedbackRequest, options CommandOptions) error {
	return r.invokeAck(ctx, operation.FeedbackCreate, request, commandOptions(options))
}
