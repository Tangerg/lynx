package execution

// RunID is the stable identity of a Run for its ENTIRE lifecycle — start, every
// interrupt/resume cycle, and its terminal — and across process restarts. It is
// never re-minted on resume.
//
// This is the load-bearing correction the rewrite makes to run identity. The
// pre-rewrite wire "runId" was a per-segment id (a fresh one on every resume,
// with a separate parentRunId chaining continuations), which conflated "the
// logical run" with "one streamed segment". Here the logical run is the RunID
// and the streamed segment is the [SegmentID]; the delivery layer projects
// whatever the wire contract needs from the two.
//
// Format: "run_<uuid>".
type RunID string

// SegmentID identifies one streamed execution segment of a Run. The initial
// start is the first segment; each resume after an interrupt opens a new one. A
// Run has one [RunID] and one-or-more Segments. Per-response event streaming and
// reconnect/replay are scoped to a Segment; the Run's lifecycle spans all of its
// Segments.
//
// Format: "seg_<...>".
type SegmentID string

// ProcessID is the agent-execution adapter's recovery handle for the process
// backing a Run — the durable snapshot key that survives a restart and lets a
// resume rehydrate the SAME process. It is an adapter detail, not a domain
// identity: nothing in the domain keys on it (the domain keys on [RunID]).
type ProcessID string
