// Package hitl implements human-in-the-loop primitives: typed Awaitable
// requests and the helper that suspends an action until a response
// arrives.
//
// The non-generic core.Awaitable lives in core/ so Process can reference
// it without dragging hitl into core's import graph; the typed layer
// here adds generic Prompt / OnResponse pairing plus a single concrete
// request shape.
package hitl

import (
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/google/uuid"
)

// Request is the typed surface every HITL prompt implements. Generic
// methods Prompt/OnResponse mirror embabel 0.4's Awaitable<P, R> contract.
// Named Request rather than Awaitable to avoid the same-name collision
// with [core.Awaitable] — the latter is the non-generic root the
// runtime uses; this one is the typed flavour user code talks to.
type Request[P any, R any] interface {
	core.Awaitable
	Prompt() P
	OnResponse(r R) core.ResponseImpact
}

// TypedRequest is the canonical [Request] implementation: an opaque ID,
// a payload of type P, and a typed response handler. One struct covers
// both the "show payload, wait for boolean confirmation" case (use
// [NewConfirmation]) and richer typed flows (use [NewTypedRequest]).
type TypedRequest[P any, R any] struct {
	IDStr   string
	Payload P
	Handler func(response R) core.ResponseImpact
}

// NewTypedRequest mints a fresh UUID for the caller — most use sites
// don't care what the ID is, only that it's stable for the duration of
// the wait.
func NewTypedRequest[P any, R any](payload P, handler func(R) core.ResponseImpact) *TypedRequest[P, R] {
	return &TypedRequest[P, R]{IDStr: uuid.NewString(), Payload: payload, Handler: handler}
}

// NewConfirmation is the boolean-response specialisation of
// [NewTypedRequest] — the ubiquitous "show payload, wait for yes/no"
// shape.
func NewConfirmation[P any](payload P, handler func(approved bool) core.ResponseImpact) *TypedRequest[P, bool] {
	return NewTypedRequest[P, bool](payload, handler)
}

func (r *TypedRequest[P, R]) ID() string     { return r.IDStr }
func (r *TypedRequest[P, R]) PromptAny() any { return r.Payload }
func (r *TypedRequest[P, R]) Prompt() P      { return r.Payload }

func (r *TypedRequest[P, R]) OnResponse(response R) core.ResponseImpact {
	if r.Handler == nil {
		return core.ResponseImpactUnchanged
	}
	return r.Handler(response)
}

// OnResponseAny implements [core.Awaitable] by type-asserting response
// to R and forwarding to [OnResponse]. Returns an error when the caller
// delivers a value of the wrong type.
func (r *TypedRequest[P, R]) OnResponseAny(response any) (core.ResponseImpact, error) {
	typed, ok := response.(R)
	if !ok {
		var zero R
		return core.ResponseImpactUnchanged,
			fmt.Errorf("hitl.TypedRequest.OnResponseAny: expected %T, got %T", zero, response)
	}
	return r.OnResponse(typed), nil
}
