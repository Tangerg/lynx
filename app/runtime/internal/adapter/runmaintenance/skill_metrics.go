package runmaintenance

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// The self-evolving skill loop is request-detached Run-boundary maintenance;
// without counters it runs blind — no client event reports how often the agent
// mines or the idle-Skill archiver archives. No-op until a MeterProvider is installed.
//
// Per-operation spans are deliberately omitted: the sibling compaction and
// extraction workers carry none, so instrumenting only the skill ops with spans
// would be inconsistent. A coherent maintenance-tracing pass is a separate
// concern; these counters cover the self-evolving loop's activity meanwhile.
const skillMetricsScope = "lynx/lyra/skill"

var minedSkillProposalCounter = sync.OnceValue(func() metric.Int64Counter {
	// A creation error yields a usable no-op counter, so it's safe to drop.
	counter, _ := otel.Meter(skillMetricsScope).Int64Counter("skill.proposals.mined",
		metric.WithDescription("Skill proposals submitted by the maintenance pipeline, by kind (new/revise)."))
	return counter
})

var archivedIdleSkillCounter = sync.OnceValue(func() metric.Int64Counter {
	counter, _ := otel.Meter(skillMetricsScope).Int64Counter("skill.idle.archived",
		metric.WithDescription("Idle agent-authored Skills archived by the maintenance pipeline."))
	return counter
})

func recordMinedSkillProposal(ctx context.Context, kind string) {
	minedSkillProposalCounter().Add(ctx, 1, metric.WithAttributes(attribute.String("skill.kind", kind)))
}

func recordArchivedIdleSkills(ctx context.Context, count int) {
	if count > 0 {
		archivedIdleSkillCounter().Add(ctx, int64(count))
	}
}
