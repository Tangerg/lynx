package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerUsage(r *Registry) {
	Query(r, MethodMeta{
		Name: "usage.session", Errors: []string{protocol.ErrSessionNotFound.Error()}, Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.SessionUsageRequest) (*protocol.Usage, error) {
		return d.api.SessionUsage(ctx, in.SessionID)
	})

	Query(r, MethodMeta{Name: "usage.summary", Stability: stable},
		func(d *Router, ctx context.Context, in protocol.UsageSummaryRequest) (*protocol.UsageSummary, error) {
			return d.api.UsageSummary(ctx, in)
		})
}
