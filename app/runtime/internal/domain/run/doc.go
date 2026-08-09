// Package run owns the product lifecycle of one logical unit of agent work.
// A Run keeps the same identity from admission through any waiting and resume
// boundaries to exactly one terminal Outcome. Each resume opens a new Segment;
// Segment identity is ephemeral stream identity while Run identity is stable.
//
// The package defines the state machine, terminal taxonomy, immutable lineage,
// admitted limits and capabilities, tree topology, and pure validation needed
// to preserve those facts. It performs no I/O and knows nothing about executor
// handles, checkpoints, conversation content, transcript ordering, or Agent Framework
// Process and Execution state.
package run
