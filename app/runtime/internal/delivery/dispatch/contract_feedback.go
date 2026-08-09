package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerFeedback(registry *Registry) {
	CommandAck(registry, MethodMeta{Name: "feedback.create", Stability: stable},
		func(router *Router, ctx context.Context, request protocol.FeedbackRequest) error {
			return router.api.CreateFeedback(ctx, request)
		})
}
