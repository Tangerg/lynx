package embedded

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// GetSessionUsage returns usage accumulated by one Session.
func (r *Runtime) GetSessionUsage(ctx context.Context, request protocol.SessionUsageRequest, options CallOptions) (*protocol.Usage, error) {
	return invoke[protocol.SessionUsageRequest, *protocol.Usage](ctx, r, "usage.session", request, callOptions(options))
}

// GetUsageSummary returns aggregated Runtime usage.
func (r *Runtime) GetUsageSummary(ctx context.Context, request protocol.UsageSummaryRequest, options CallOptions) (*protocol.UsageSummary, error) {
	return invoke[protocol.UsageSummaryRequest, *protocol.UsageSummary](ctx, r, "usage.summary", request, callOptions(options))
}
