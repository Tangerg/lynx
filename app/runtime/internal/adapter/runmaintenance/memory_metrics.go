package runmaintenance

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// The agent-memory consolidation fold is request-detached Run-boundary
// maintenance; without counters it runs blind — no client event reports how
// often facts are extracted into the ledger or a curated generation is published.
// These give the memory pipeline the same observability the skill and goal
// loops already carry. No-op until a MeterProvider is installed.
//
// Per-operation spans are deliberately omitted, matching skill_metrics and the
// un-instrumented compaction worker: instrumenting only some maintenance ops
// with spans would be inconsistent. A coherent maintenance-tracing pass is a
// separate concern.
const memoryMetricsScope = "lynx/lyra/memory"

var extractedFactCounter = sync.OnceValue(func() metric.Int64Counter {
	// A creation error yields a usable no-op counter, so it's safe to drop.
	counter, _ := otel.Meter(memoryMetricsScope).Int64Counter("memory.facts.extracted",
		metric.WithDescription("Durable facts the consolidator appended to the daily ledger."))
	return counter
})

var publishedMemoryGenerationCounter = sync.OnceValue(func() metric.Int64Counter {
	counter, _ := otel.Meter(memoryMetricsScope).Int64Counter("memory.generations.published",
		metric.WithDescription("Curated memory generations the fold published (a watermark advance)."))
	return counter
})

func recordExtractedFacts(ctx context.Context, count int) {
	if count > 0 {
		extractedFactCounter().Add(ctx, int64(count))
	}
}

func recordPublishedMemoryGeneration(ctx context.Context) {
	publishedMemoryGenerationCounter().Add(ctx, 1)
}
