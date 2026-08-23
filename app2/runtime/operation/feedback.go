package operation

import (
	"context"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

func registerFeedback(registry *Registry) {
	CommandAck(registry, MethodMeta{
		Name: "feedback.create",
		Errors: []string{
			protocol.ErrSessionNotFound.Error(),
			protocol.ErrRunNotFound.Error(),
			protocol.ErrItemNotFound.Error(),
		},
	},
		func(service interface {
			CreateFeedback(context.Context, protocol.FeedbackRequest) error
		}, ctx context.Context, request protocol.FeedbackRequest) error {
			return service.CreateFeedback(ctx, request)
		})
}
