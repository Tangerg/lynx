package embedded

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// GetSessionUsage returns usage accumulated by one Session.
func (r *Runtime) GetSessionUsage(ctx context.Context, request protocol.SessionUsageRequest, options CallOptions) (*protocol.Usage, error) {
	return r.invoke[protocol.SessionUsageRequest, *protocol.Usage](ctx, operation.UsageSession, request, callOptions(options))
}

// GetUsageSummary returns aggregated Runtime usage.
func (r *Runtime) GetUsageSummary(ctx context.Context, request protocol.UsageSummaryRequest, options CallOptions) (*protocol.UsageSummary, error) {
	return r.invoke[protocol.UsageSummaryRequest, *protocol.UsageSummary](ctx, operation.UsageSummary, request, callOptions(options))
}
