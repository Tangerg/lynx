// Package hitl implements human-in-the-loop primitives: the linear
// Interrupt[R] guard plus typed Awaitable helpers for explicit park /
// resume flows.
//
// The non-generic core.Awaitable lives in core/ so Process can reference
// it without dragging hitl into core's import graph; the typed layer
// here adds generic Prompt / OnResponse pairing plus a single concrete
// request shape used by both explicit awaitables and Interrupt.
package hitl
