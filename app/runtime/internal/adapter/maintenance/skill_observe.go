package maintenance

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// The self-evolving skill loop is request-detached turn-boundary maintenance;
// without counters it runs blind — no client event reports how often the agent
// mines or the curator prunes. No-op until a MeterProvider is installed.
//
// Per-operation spans are deliberately omitted: the sibling compaction and
// extraction workers carry none, so instrumenting only the skill ops with spans
// would be inconsistent. A coherent maintenance-tracing pass is a separate
// concern; these counters cover the self-evolving loop's activity meanwhile.
const skillObserveScope = "lynx/lyra/skill"

var loadMinedSkills = sync.OnceValue(func() metric.Int64Counter {
	// A creation error yields a usable no-op counter, so it's safe to drop.
	counter, _ := otel.Meter(skillObserveScope).Int64Counter("skill.drafts.mined",
		metric.WithDescription("Skill drafts the miner staged, by kind (new/revise)."))
	return counter
})

var loadArchivedSkills = sync.OnceValue(func() metric.Int64Counter {
	counter, _ := otel.Meter(skillObserveScope).Int64Counter("skill.curated.archived",
		metric.WithDescription("Idle agent-authored skills the curator archived."))
	return counter
})

func recordMinedSkill(ctx context.Context, kind string) {
	loadMinedSkills().Add(ctx, 1, metric.WithAttributes(attribute.String("skill.kind", kind)))
}

func recordArchivedSkills(ctx context.Context, count int) {
	if count > 0 {
		loadArchivedSkills().Add(ctx, int64(count))
	}
}
