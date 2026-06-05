package hitl

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/agent/core"
)

// Request is the typed surface every HITL prompt implements. Generic
// methods Prompt/OnResponse mirror embabel 0.4's Awaitable<P, R> contract.
// Named Request rather than Awaitable to avoid the same-name collision
// with [core.Awaitable] — the latter is the non-generic root the
// runtime uses; this one is the typed flavor user code talks to.
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
	id      string
	payload P
	handler func(response R) core.ResponseImpact
}

// NewTypedRequest mints a fresh UUID for the caller — most use sites
// don't care what the ID is, only that it's stable for the duration of
// the wait.
func NewTypedRequest[P any, R any](payload P, handler func(R) core.ResponseImpact) *TypedRequest[P, R] {
	return &TypedRequest[P, R]{id: uuid.NewString(), payload: payload, handler: handler}
}

// NewConfirmation is the boolean-response specialisation of
// [NewTypedRequest] — the ubiquitous "show payload, wait for yes/no"
// shape.
func NewConfirmation[P any](payload P, handler func(approved bool) core.ResponseImpact) *TypedRequest[P, bool] {
	return NewTypedRequest[P, bool](payload, handler)
}

func (r *TypedRequest[P, R]) ID() string     { return r.id }
func (r *TypedRequest[P, R]) PromptAny() any { return r.payload }
func (r *TypedRequest[P, R]) Prompt() P      { return r.payload }

func (r *TypedRequest[P, R]) OnResponse(response R) core.ResponseImpact {
	if r.handler == nil {
		return core.ImpactUnchanged
	}
	return r.handler(response)
}

// OnResponseAny implements [core.Awaitable] by type-asserting response
// to R and forwarding to [OnResponse]. Returns an error when the caller
// delivers a value of the wrong type.
func (r *TypedRequest[P, R]) OnResponseAny(response any) (core.ResponseImpact, error) {
	typed, ok := response.(R)
	if !ok {
		var zero R
		return core.ImpactUnchanged,
			fmt.Errorf("deliver response: expected %T, got %T", zero, response)
	}
	return r.OnResponse(typed), nil
}
