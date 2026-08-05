package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerFeedback(r *Registry) {
	CommandAck(r, MethodMeta{Name: "feedback.create", Stability: stable},
		func(d *Router, ctx context.Context, in protocol.FeedbackRequest) error {
			return d.api.CreateFeedback(ctx, in)
		})
}
