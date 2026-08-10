package operation

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func registerUsage(registry *Registry) {
	Query(registry, MethodMeta{
		Name: "usage.session", Errors: []string{protocol.ErrSessionNotFound.Error()}, Stability: stable,
	}, func(service Service, ctx context.Context, request protocol.SessionUsageRequest) (*protocol.Usage, error) {
		return service.SessionUsage(ctx, request.SessionID)
	})

	Query(registry, MethodMeta{Name: "usage.summary", Stability: stable},
		func(service Service, ctx context.Context, request protocol.UsageSummaryRequest) (*protocol.UsageSummary, error) {
			return service.UsageSummary(ctx, request)
		})
}
