// Package runs owns a Run's complete application lifecycle: admission, live
// registry, per-Segment execution, event publication, interrupt persistence,
// and terminal commit. Reading this package should explain a Run from Start to
// terminal without depending on an outer presentation or transport.
//
// It builds on the execution domain vocabulary (RunID / SegmentID / RunState /
// Outcome) and defines the ports it consumes (executor, store) —
// interfaces owned by the consumer and satisfied structurally by composition.
//
// The pieces: the [Journal] (per-run event fan-out + bounded replay), the live
// registry (single-writer admission + run records), and the per-segment pump
// that drains an executor's events into the journal — all coordinated by
// [Coordinator].
package runs
