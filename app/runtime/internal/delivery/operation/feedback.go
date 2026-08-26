package operation

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

const FeedbackCreate Name = "feedback.create"

func registerFeedback(registry *Registry) {
	registry.CommandAck(MethodMeta{Name: FeedbackCreate},
		func(service interface {
			CreateFeedback(context.Context, protocol.FeedbackRequest) error
		}, ctx context.Context, request protocol.FeedbackRequest) error {
			return service.CreateFeedback(ctx, request)
		})
}
