// Package hitl implements human-in-the-loop primitives: typed Awaitable
// requests and the helper that suspends an action until a response
// arrives.
//
// The non-generic core.Awaitable lives in core/ so Process can reference
// it without dragging hitl into core's import graph; the typed layer
// here adds generic Prompt / OnResponse pairing plus a single concrete
// request shape.
package hitl
