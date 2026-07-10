// Package execution is the core bounded context of the runtime: the lifecycle
// of a Run — the whole logical execution the user thinks of as "the agent
// working on my request", from its first streamed output through any
// interrupt/resume cycles to a single terminal outcome.
//
// # Ubiquitous language
//
// These words have ONE meaning across the runtime; they are the vocabulary the
// rewrite converges every layer onto (delivery, application, adapters):
//
//   - Session   — a durable conversation with an agent. Owns working-tree
//     binding, title, default model, and fork/subtask lineage. A Session may
//     admit at most one non-terminal Run at a time. (Identity: session.IDPrefix.)
//   - Run       — one logical execution within a Session. Has a STABLE [RunID]
//     for its entire lifetime: start, every interrupt/resume, and its terminal —
//     including across process restarts. The Run is what carries lifecycle
//     [RunState], budget, usage, and terminal [Outcome].
//   - Segment   — one streamed execution of a Run. The initial start is the
//     first segment; each resume after an interrupt opens a NEW segment. A Run
//     has one RunID and one-or-more Segments ([SegmentID]). Reconnect/replay and
//     the per-response event stream are per-Segment; lifecycle is per-Run.
//   - Step      — one model/tool round inside a segment. The wire surfaces it as
//     a count (steps / maxSteps); it is not a first-class identity.
//   - Process   — the agent-execution adapter's backing process. Its [ProcessID]
//     is a recovery handle (the durable snapshot key that survives restart), NOT
//     a domain identity: the domain keys on RunID.
//
// "Turn" is deliberately retired from the architecture language. What the old
// kernel called a turn is a Run; what it streamed per start/resume is a Segment;
// a single model/tool round is a Step.
//
// # What lives here
//
// This package holds only what the runtime must PROTECT: the Run state machine
// and its legal transitions ([RunState]), the terminal-reason taxonomy
// ([Outcome]) that both the executor's terminal decision and the wire projection
// resolve against, the identity value types and their stability contracts, and
// the durable-vs-live event classification with its commit-before-publish
// ordering rule ([Durability]). It is pure: no I/O, no storage, no wire types,
// no agent SDK — those are outer rings that depend inward on this vocabulary.
package execution
