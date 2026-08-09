// Package runs owns the complete Run application lifecycle: admission, neutral
// executor coordination, per-Segment fact reduction, durable publication,
// waiting continuation, cancellation, recovery, and terminalization. It defines
// the narrow ports each use case consumes while domain Run values retain their
// own lifecycle and topology invariants. [Coordinator] composes these behaviors
// without depending on presentation, transport, or a concrete executor.
package runs
