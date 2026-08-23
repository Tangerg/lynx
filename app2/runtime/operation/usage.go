package operation

import (
	"context"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

func registerUsage(registry *Registry) {
	Query(registry, MethodMeta{
		Name: "usage.session", Errors: []string{protocol.ErrSessionNotFound.Error()},
	}, func(service interface {
		SessionUsage(context.Context, string) (*protocol.Usage, error)
	}, ctx context.Context, request protocol.SessionUsageRequest) (*protocol.Usage, error) {
		return service.SessionUsage(ctx, request.SessionID)
	})

	Query(registry, MethodMeta{Name: "usage.summary"},
		func(service interface {
			UsageSummary(context.Context, protocol.UsageSummaryRequest) (*protocol.UsageSummary, error)
		}, ctx context.Context, request protocol.UsageSummaryRequest) (*protocol.UsageSummary, error) {
			return service.UsageSummary(ctx, request)
		})
}
