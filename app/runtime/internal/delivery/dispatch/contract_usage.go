package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func registerUsage(registry *Registry) {
	Query(registry, MethodMeta{
		Name: "usage.session", Errors: []string{protocol.ErrSessionNotFound.Error()}, Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.SessionUsageRequest) (*protocol.Usage, error) {
		return router.api.SessionUsage(ctx, request.SessionID)
	})

	Query(registry, MethodMeta{Name: "usage.summary", Stability: stable},
		func(router *Router, ctx context.Context, request protocol.UsageSummaryRequest) (*protocol.UsageSummary, error) {
			return router.api.UsageSummary(ctx, request)
		})
}
