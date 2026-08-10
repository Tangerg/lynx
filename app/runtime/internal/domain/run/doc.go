// Package run owns the product lifecycle of one logical unit of agent work.
// A Run keeps the same identity from admission through any waiting and resume
// boundaries to exactly one terminal Outcome. Each resume opens a new Segment;
// Segment identity is ephemeral stream identity while Run identity is stable.
//
// The package owns the complete Run aggregate: identity, state machine,
// terminal taxonomy, failure, cumulative metrics, immutable lineage, admitted
// limits and capabilities, and tree topology. It performs no I/O and knows
// nothing about executor handles or state, checkpoints, conversation content,
// open interrupts, or transcript ordering.
package run
