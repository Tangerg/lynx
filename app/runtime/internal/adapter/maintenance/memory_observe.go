package maintenance

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// The agent-memory extract/curate fold is request-detached turn-boundary
// maintenance; without counters it runs blind — no client event reports how
// often facts are mined into the ledger or a curated generation is published.
// These give the memory pipeline the same observability the skill and goal
// loops already carry. No-op until a MeterProvider is installed.
//
// Per-operation spans are deliberately omitted, matching skill_observe and the
// un-instrumented compaction worker: instrumenting only some maintenance ops
// with spans would be inconsistent. A coherent maintenance-tracing pass is a
// separate concern.
const memoryObserveScope = "lynx/lyra/memory"

var loadMinedFacts = sync.OnceValue(func() metric.Int64Counter {
	// A creation error yields a usable no-op counter, so it's safe to drop.
	counter, _ := otel.Meter(memoryObserveScope).Int64Counter("memory.facts.mined",
		metric.WithDescription("Durable facts the extractor appended to the daily ledger."))
	return counter
})

var loadCuratedGenerations = sync.OnceValue(func() metric.Int64Counter {
	counter, _ := otel.Meter(memoryObserveScope).Int64Counter("memory.curated.published",
		metric.WithDescription("Curated memory generations the fold published (a watermark advance)."))
	return counter
})

func recordMinedFacts(ctx context.Context, count int) {
	if count > 0 {
		loadMinedFacts().Add(ctx, int64(count))
	}
}

func recordCuratedGeneration(ctx context.Context) {
	loadCuratedGenerations().Add(ctx, 1)
}
