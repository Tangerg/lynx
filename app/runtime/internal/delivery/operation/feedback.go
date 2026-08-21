package operation

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func registerFeedback(registry *Registry) {
	CommandAck(registry, MethodMeta{Name: "feedback.create"},
		func(service interface {
			CreateFeedback(context.Context, protocol.FeedbackRequest) error
		}, ctx context.Context, request protocol.FeedbackRequest) error {
			return service.CreateFeedback(ctx, request)
		})
}
