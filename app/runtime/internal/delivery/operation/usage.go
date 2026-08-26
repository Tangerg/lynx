package operation

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

const (
	UsageSession Name = "usage.session"
	UsageSummary Name = "usage.summary"
)

func registerUsage(registry *Registry) {
	registry.Query(MethodMeta{
		Name: UsageSession, Errors: []string{protocol.ErrSessionNotFound.Error()},
	}, func(service interface {
		SessionUsage(context.Context, string) (*protocol.Usage, error)
	}, ctx context.Context, request protocol.SessionUsageRequest) (*protocol.Usage, error) {
		return service.SessionUsage(ctx, request.SessionID)
	})

	registry.Query(MethodMeta{Name: UsageSummary},
		func(service interface {
			UsageSummary(context.Context, protocol.UsageSummaryRequest) (*protocol.UsageSummary, error)
		}, ctx context.Context, request protocol.UsageSummaryRequest) (*protocol.UsageSummary, error) {
			return service.UsageSummary(ctx, request)
		})
}
