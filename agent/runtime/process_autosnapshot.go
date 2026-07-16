package runtime

import (
	"context"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Tangerg/lynx/agent/event"
)

// SnapshotFailurePolicy controls automatic durability failure behavior.
type SnapshotFailurePolicy uint8

const (
	SnapshotFailureFailProcess SnapshotFailurePolicy = iota
	SnapshotFailurePauseProcess
	SnapshotFailureReportOnly
)

func (p SnapshotFailurePolicy) Valid() bool {
	return p == SnapshotFailureFailProcess || p == SnapshotFailurePauseProcess || p == SnapshotFailureReportOnly
}

func (p SnapshotFailurePolicy) String() string {
	switch p {
	case SnapshotFailureFailProcess:
		return "fail_process"
	case SnapshotFailurePauseProcess:
		return "pause_process"
	case SnapshotFailureReportOnly:
		return "report_only"
	default:
		return "unknown"
	}
}

func (p *Process) maybeAutoSnapshot(ctx context.Context) error {
	if p.engine == nil || !p.engine.autoSnapshot || p.engine.processStore == nil {
		return nil
	}

	_, err := p.engine.saveProcess(ctx, p)
	if err == nil {
		return nil
	}
	_, span := agentTracer.Start(ctx, "agent.auto_snapshot")
	span.SetAttributes(attribute.String(attrProcessID, p.id))
	finishSpanWithError(span, err)
	span.End()
	policy := p.engine.snapshotFailurePolicy
	p.publishEvent(ctx, event.ProcessSnapshotFailed{
		Header: p.eventHeader(),
		Policy: policy.String(),
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
