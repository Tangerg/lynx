package operation

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func registerFeedback(registry *Registry) {
	CommandAck(registry, MethodMeta{Name: "feedback.create", Stability: stable},
		func(service Service, ctx context.Context, request protocol.FeedbackRequest) error {
			return service.CreateFeedback(ctx, request)
		})
}
