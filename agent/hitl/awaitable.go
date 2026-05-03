// Package hitl implements human-in-the-loop primitives: typed Awaitable
// requests and the helper that suspends an action until a response
// arrives.
//
// The non-generic core.Awaitable lives in core/ so Process can reference
// it without dragging hitl into core's import graph; the typed layer here
// adds generic Prompt / OnResponse pairing plus concrete request shapes.
package hitl

import (
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/google/uuid"
)

// Request is the typed surface every HITL prompt implements. Generic
// methods Prompt/OnResponse mirror embabel 0.4's Awaitable<P, R> contract.
// Named Request rather than Awaitable to avoid the same-name collision
// with [core.Awaitable] — the latter is the non-generic root that the
// runtime uses; this one is the typed flavour user code talks to.
type Request[P any, R any] interface {
	core.Awaitable
	Prompt() P
	OnResponse(r R) core.ResponseImpact
}

// ConfirmationRequest is the simplest Awaitable — show a payload, wait for
// a yes/no.
type ConfirmationRequest[P any] struct {
	IDStr   string
	Payload P
	Handler func(approved bool) core.ResponseImpact
}

// NewConfirmationRequest mints an ID for the caller — most use sites don't
// care what the ID is, only that it's stable for the duration of the wait.
func NewConfirmationRequest[P any](payload P, handler func(approved bool) core.ResponseImpact) *ConfirmationRequest[P] {
	return &ConfirmationRequest[P]{IDStr: uuid.NewString(), Payload: payload, Handler: handler}
}

func (c *ConfirmationRequest[P]) ID() string     { return c.IDStr }
func (c *ConfirmationRequest[P]) PromptAny() any { return c.Payload }
func (c *ConfirmationRequest[P]) Prompt() P      { return c.Payload }

func (c *ConfirmationRequest[P]) OnResponse(approved bool) core.ResponseImpact {
	if c.Handler == nil {
		return core.ResponseImpactUnchanged
	}
	return c.Handler(approved)
}

// OnResponseAny implements [core.Awaitable] by type-asserting response
// to bool and forwarding to [OnResponse]. Returns an error when the
// caller delivers a non-bool value.
func (c *ConfirmationRequest[P]) OnResponseAny(response any) (core.ResponseImpact, error) {
	approved, ok := response.(bool)
	if !ok {
		return core.ResponseImpactUnchanged,
			fmt.Errorf("hitl.ConfirmationRequest: expected bool response, got %T", response)
	}
	return c.OnResponse(approved), nil
}

// FormBindingRequest is a richer awaitable: a typed prompt plus a typed
// reply callback. Concrete schema definitions live with the host
// application.
type FormBindingRequest[P any, R any] struct {
	IDStr   string
	Payload P
	Handler func(response R) core.ResponseImpact
}

// NewFormBindingRequest creates a FormBindingRequest with a fresh UUID.
func NewFormBindingRequest[P any, R any](payload P, handler func(R) core.ResponseImpact) *FormBindingRequest[P, R] {
	return &FormBindingRequest[P, R]{IDStr: uuid.NewString(), Payload: payload, Handler: handler}
}

func (f *FormBindingRequest[P, R]) ID() string     { return f.IDStr }
func (f *FormBindingRequest[P, R]) PromptAny() any { return f.Payload }
func (f *FormBindingRequest[P, R]) Prompt() P      { return f.Payload }

func (f *FormBindingRequest[P, R]) OnResponse(response R) core.ResponseImpact {
	if f.Handler == nil {
		return core.ResponseImpactUnchanged
	}
	return f.Handler(response)
}

// OnResponseAny implements [core.Awaitable] by type-asserting response
// to R and forwarding to [OnResponse]. Returns an error when the
// caller delivers a value of the wrong type.
func (f *FormBindingRequest[P, R]) OnResponseAny(response any) (core.ResponseImpact, error) {
	typed, ok := response.(R)
	if !ok {
		var zero R
		return core.ResponseImpactUnchanged,
			fmt.Errorf("hitl.FormBindingRequest: expected response of type %T, got %T", zero, response)
	}
	return f.OnResponse(typed), nil
}
