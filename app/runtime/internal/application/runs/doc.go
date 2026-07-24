// Package runs is the application layer that owns a Run's lifecycle — the live
// machinery the pre-rewrite code scattered across the delivery server (event
// hub, run registry, per-segment pump) and the kernel/runtime (admission,
// interrupt persistence, terminal commit). Centralizing it here is the pivot of
// the execution-centered rewrite: reading this package should explain a Run from
// Start to terminal, and the delivery layer should only translate this package's
// transport-neutral output to the wire.
//
// It builds on the execution domain vocabulary (RunID / SegmentID / RunState /
// Outcome) and defines the ports it consumes (executor, store) —
// interfaces owned by the consumer, satisfied structurally by the adapters the
// composition root injects.
//
// The pieces: the [Journal] (per-run event fan-out + durable replay), the live
// registry (single-writer admission + run records), and the per-segment pump
// that drains an executor's events into the journal — all coordinated by
// [Coordinator]. Delivery keeps only wire framing on top.
package runs
