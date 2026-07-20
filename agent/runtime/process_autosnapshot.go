package runtime

import (
	"context"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
)

// SnapshotFailurePolicy controls automatic durability failure behavior.
type SnapshotFailurePolicy = core.SnapshotFailurePolicy

const (
	SnapshotFailureFailProcess  = core.SnapshotFailureFailProcess
	SnapshotFailurePauseProcess = core.SnapshotFailurePauseProcess
	SnapshotFailureReportOnly   = core.SnapshotFailureReportOnly
)

func (p *Process) maybeAutoSnapshot(ctx context.Context) error {
	if p.engine == nil || !p.engine.autoSnapshot || p.engine.processStore == nil {
		return nil
	}

	_, err := p.engine.saveProcess(ctx, p)
	if err == nil {
		return nil
	}
	_, span := agentTracer.Start(ctx, spanAutoSnapshot)
	span.SetAttributes(attribute.String(attrProcessID, p.id))
	finishSpanWithError(span, err)
	span.End()
	policy := p.engine.snapshotFailurePolicy
	p.publishEvent(ctx, event.ProcessSnapshotFailed{
		Header: p.eventHeader(),
		Policy: policy,
		Err:    err,
	})

	switch policy {
	case SnapshotFailureReportOnly:
		return nil
	case SnapshotFailurePauseProcess:
		p.state.pauseDurability()
		return nil
	default:
		p.state.failDurability(err)
		return err
	}
}
