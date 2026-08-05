package agent2

import (
	"context"
	"encoding/json"
)

// ReplayPolicy states whether a Dispatcher can prove that repeating an Effect
// with the same EffectID is the same logical external operation. It does not
// claim transactionality or allow replay under a different identity.
type ReplayPolicy uint8

const (
	ReplayPolicyInvalid ReplayPolicy = iota
	ReplayPolicyNever
	ReplayPolicySameIdentity
)

func (policy ReplayPolicy) String() string {
	switch policy {
	case ReplayPolicyNever:
		return "never"
	case ReplayPolicySameIdentity:
		return "same_identity"
	default:
		return "invalid"
	}
}

// EffectRequest is the immutable dispatch context prepared by the Engine.
type EffectRequest struct {
	processID ProcessID
	step      uint64
	index     uint32
	id        EffectID
	effect    Effect
}

func newEffectRequest(processID ProcessID, step uint64, index uint32, id EffectID, effect Effect) EffectRequest {
	return EffectRequest{processID: processID, step: step, index: index, id: id, effect: effect.clone()}
}

// ProcessID returns the Process that owns the Effect.
func (request EffectRequest) ProcessID() ProcessID { return request.processID }

// StepSequence returns the one-based Step sequence that declared the Effect.
func (request EffectRequest) StepSequence() uint64 { return request.step }

// Index returns the zero-based declaration order within the Step Effect batch.
func (request EffectRequest) Index() uint32 { return request.index }

// ID returns the stable identity assigned during Step preparation.
func (request EffectRequest) ID() EffectID { return request.id }

// Effect returns an independently owned copy of the frozen intent.
func (request EffectRequest) Effect() Effect { return request.effect.clone() }

func (request EffectRequest) valid() bool {
	return request.processID.Valid() && request.step > 0 && request.id.Valid() && request.effect.Valid()
}

// DeltaEmitter accepts Strategy-owned streaming payloads while Dispatch is
// active. The Engine validates, orders, bounds, and publishes each payload as a
// best-effort Delta. It intentionally returns no observer error. A Dispatcher
// must not retain or call it after Dispatch returns.
type DeltaEmitter func(json.RawMessage)

// Dispatcher executes Strategy-owned Effects outside Execution.Step. It must
// return a Settlement addressed to request.ID. A returned error means the Engine
// cannot prove the external result and records an unknown settlement. The same
// Dispatcher may serve Processes concurrently; implementations must be
// concurrency-safe, return in bounded time, not mutate an Execution, and not
// start unowned goroutines. ReplayPolicy must be a pure, deterministic
// declaration for the supplied immutable Effect.
type Dispatcher interface {
	Dispatch(context.Context, EffectRequest, DeltaEmitter) (Settlement, error)
	ReplayPolicy(effect Effect) ReplayPolicy
}
