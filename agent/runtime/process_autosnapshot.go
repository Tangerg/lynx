package runtime

import (
	"context"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Tangerg/lynx/agent/core"
)

// maybeAutoSnapshot persists the current process state when the platform
// has auto-snapshot enabled and a store configured. Best-effort: a
// persistence failure is recorded on a span but never aborts the running
// process — losing a snapshot is recoverable, killing a live agent is not.
func (p *AgentProcess) maybeAutoSnapshot(ctx context.Context) {
	if p.platform == nil || !p.platform.autoSnapshot || p.platform.processStore == nil {
		return
	}

	if err := p.platform.processStore.Save(ctx, p.Snapshot()); err != nil {
		_, span := core.AgentTracer().Start(ctx, "agent.auto_snapshot")
		span.SetAttributes(attribute.String(attrProcessID, p.id))
		finishSpanWithError(span, err)
		span.End()
	}
}
