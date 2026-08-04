// Package execution is the core bounded context of the runtime: the lifecycle
// of a Run — the whole logical execution the user thinks of as "the agent
// working on my request", from its first streamed output through any
// interrupt/resume cycles to a single terminal outcome.
//
// # Ubiquitous language
//
// These words have one meaning throughout the execution context:
//
//   - Session   — a durable conversation with an agent. Owns working-tree
//     binding, title, default model, and fork/subtask lineage. A Session may
//     admit at most one non-terminal root Run tree at a time. (Identity:
//     session.IDPrefix.)
//   - Run       — one logical execution within a Session. Has a STABLE [RunID]
//     for its entire lifetime: start, every interrupt/resume, and its terminal —
//     including across process restarts. The Run is what carries lifecycle
//     [RunState], budget, usage, and terminal [Outcome].
//   - Segment   — one streamed execution of a Run. The initial start is the
//     first segment; each resume after an interrupt opens a NEW segment. A Run
//     has one RunID and one-or-more Segments ([SegmentID]). Reconnect/replay and
//     the per-response event stream are per-Segment; lifecycle is per-Run.
//   - Step      — one model/tool round inside a Segment. It is counted but has no
//     first-class identity.
//   - Process   — one executor member. Its [ProcessID] is a recovery and routing
//     handle, while durable product lifecycle keys on RunID.
//
// # What lives here
//
// This package protects the Run state machine and its legal transitions
// ([RunState]), the terminal-reason taxonomy ([Outcome]), and the identity value
// types and their stability contracts. It performs no I/O.
//
// # Co-located sub-contexts
//
// The execution context is ONE bounded context but not one aggregate (§4.1): the
// run-scoped state and projections that co-evolve with a Run live as sub-packages
// under this directory rather than as independent top-level domains —
// execution/interrupts (the open-HITL registry, a Run state), execution/transcript
// (the durable Item history + rollback/fork boundary), execution/conversation (the
// chat-message log), and execution/accounting (token/cost usage value objects).
// Each is its own package with its own model; the nesting expresses that they
// belong to the Run's lifecycle, not that they share code.
package execution
