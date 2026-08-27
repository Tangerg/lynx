package agentexec

import (
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// recalledMemoryTopK bounds how many relevant memory items the per-turn recall
// block surfaces. Small on purpose — the pinned core is always present; this is
// the "what's relevant right now" supplement.
const recalledMemoryTopK = 5

const memoryScope = "scope/lyra/memory"

var recallTracer = otel.Tracer(memoryScope)

var loadRecallCounter = sync.OnceValue(func() metric.Int64Counter {
	// A creation error yields a usable no-op counter, so it's safe to drop.
	counter, _ := otel.Meter(memoryScope).Int64Counter("memory.recalled",
		metric.WithDescription("Non-pinned memory items retrieved and injected as a turn's recall block."))
	return counter
})
