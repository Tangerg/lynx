package agenttest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"

	agent "github.com/Tangerg/lynx/agent"
)

var (
	// ErrInvalidDispatchScript reports a malformed deterministic dispatch script.
	ErrInvalidDispatchScript = errors.New("agenttest: invalid dispatch script")
	// ErrUnexpectedDispatch reports an Effect beyond the configured script.
	ErrUnexpectedDispatch = errors.New("agenttest: unexpected dispatch")
	// ErrEffectMismatch reports an Effect that differs from the next expectation.
	ErrEffectMismatch = errors.New("agenttest: effect does not match script")
)

// DispatchStep describes one expected Dispatcher call and its deterministic
// stream and settlement outcome. ExpectedEffect is optional; when present, the
// complete immutable Effect must match. Error may follow emitted Deltas and is
// mutually exclusive with SettlementStatus and SettlementPayload.
type DispatchStep struct {
	// ExpectedEffect is the optional complete Effect expected at this step.
	ExpectedEffect *agent.Effect
	// Deltas are emitted in declaration order before the final outcome.
	Deltas []json.RawMessage
	// SettlementStatus is the definite or unknown settlement status.
	SettlementStatus agent.SettlementStatus
	// SettlementPayload is the Strategy-owned settlement payload.
	SettlementPayload json.RawMessage
	// Error makes Dispatch return an indeterminate external error.
	Error error
}

// ScriptedDispatcherConfig declares a finite deterministic dispatch script.
type ScriptedDispatcherConfig struct {
	// ReplayPolicy is returned for every valid Dispatcher Effect.
	ReplayPolicy agent.ReplayPolicy
	// Steps are consumed in actual Dispatch order.
	Steps []DispatchStep
}

type dispatchStep struct {
	expectedEffectJSON []byte
	deltas             []json.RawMessage
	settlementStatus   agent.SettlementStatus
	settlementPayload  json.RawMessage
	err                error
}

// ScriptedDispatcher is a concurrency-safe finite Dispatcher fixture. It
// records every request that reaches script consumption and fails calls made
// beyond or contrary to the configured script.
type ScriptedDispatcher struct {
	replayPolicy agent.ReplayPolicy

	mu       sync.Mutex
	steps    []dispatchStep
	next     int
	requests []agent.EffectRequest
}

// NewScriptedDispatcher validates and freezes config.
func NewScriptedDispatcher(config ScriptedDispatcherConfig) (*ScriptedDispatcher, error) {
	if config.ReplayPolicy != agent.ReplayPolicyNever &&
		config.ReplayPolicy != agent.ReplayPolicySameIdentity {
		return nil, fmt.Errorf("%w: ReplayPolicy is required", ErrInvalidDispatchScript)
	}
	steps := make([]dispatchStep, len(config.Steps))
	for index, source := range config.Steps {
		step, err := freezeDispatchStep(source)
		if err != nil {
			return nil, fmt.Errorf("%w: Steps[%d]: %w", ErrInvalidDispatchScript, index, err)
		}
		steps[index] = step
	}
	return &ScriptedDispatcher{replayPolicy: config.ReplayPolicy, steps: steps}, nil
}

func freezeDispatchStep(source DispatchStep) (dispatchStep, error) {
	step := dispatchStep{settlementStatus: source.SettlementStatus, err: source.Error}
	if source.ExpectedEffect != nil {
		if !source.ExpectedEffect.Valid() {
			return dispatchStep{}, agent.ErrInvalidEffect
		}
		encoded, err := json.Marshal(*source.ExpectedEffect)
		if err != nil {
			return dispatchStep{}, fmt.Errorf("encode expected Effect: %w", err)
		}
		step.expectedEffectJSON = encoded
	}
	step.deltas = make([]json.RawMessage, len(source.Deltas))
	for index, delta := range source.Deltas {
		if !json.Valid(delta) {
			return dispatchStep{}, fmt.Errorf("deltas[%d] is not valid JSON", index)
		}
		step.deltas[index] = bytes.Clone(delta)
	}
	if source.Error != nil {
		if source.SettlementStatus != agent.SettlementStatusInvalid || len(source.SettlementPayload) != 0 {
			return dispatchStep{}, errors.New("error cannot be combined with a settlement")
		}
		return step, nil
	}
	if source.SettlementStatus < agent.SettlementStatusSucceeded ||
		source.SettlementStatus > agent.SettlementStatusUnknown {
		return dispatchStep{}, errors.New("settlement status is required")
	}
	if !json.Valid(source.SettlementPayload) {
		return dispatchStep{}, errors.New("settlement payload is not valid JSON")
	}
	step.settlementPayload = bytes.Clone(source.SettlementPayload)
	return step, nil
}

// Dispatch consumes the next scripted step, emits its Deltas, and returns its
// configured settlement or error.
func (dispatcher *ScriptedDispatcher) Dispatch(
	ctx context.Context,
	request agent.EffectRequest,
	emit agent.DeltaEmitter,
) (agent.Settlement, error) {
	if err := ctx.Err(); err != nil {
		return agent.Settlement{}, err
	}
	if dispatcher == nil {
		return agent.Settlement{}, ErrInvalidDispatchScript
	}
	step, err := dispatcher.consume(request)
	if err != nil {
		return agent.Settlement{}, err
	}
	return step.dispatch(request, emit)
}

func (dispatcher *ScriptedDispatcher) consume(request agent.EffectRequest) (dispatchStep, error) {
	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	dispatcher.requests = append(dispatcher.requests, request)
	if dispatcher.next >= len(dispatcher.steps) {
		return dispatchStep{}, ErrUnexpectedDispatch
	}
	step := dispatcher.steps[dispatcher.next]
	dispatcher.next++
	return step, nil
}

func (step dispatchStep) dispatch(
	request agent.EffectRequest,
	emit agent.DeltaEmitter,
) (agent.Settlement, error) {
	matches, err := step.matches(request.Effect())
	if err != nil {
		return agent.Settlement{}, fmt.Errorf("%w: %w", ErrEffectMismatch, err)
	}
	if !matches {
		return agent.Settlement{}, ErrEffectMismatch
	}
	if len(step.deltas) > 0 && emit == nil {
		return agent.Settlement{}, fmt.Errorf("%w: nil DeltaEmitter", ErrUnexpectedDispatch)
	}
	for _, delta := range step.deltas {
		emit(bytes.Clone(delta))
	}
	if step.err != nil {
		return agent.Settlement{}, step.err
	}
	return agent.NewSettlement(request.ID(), step.settlementStatus, step.settlementPayload)
}

func (step dispatchStep) matches(effect agent.Effect) (bool, error) {
	if step.expectedEffectJSON == nil {
		return true, nil
	}
	actual, err := json.Marshal(effect)
	if err != nil {
		return false, err
	}
	return bytes.Equal(step.expectedEffectJSON, actual), nil
}

// ReplayPolicy returns the immutable policy declared at construction.
func (dispatcher *ScriptedDispatcher) ReplayPolicy(effect agent.Effect) agent.ReplayPolicy {
	if dispatcher == nil || !effect.Valid() {
		return agent.ReplayPolicyInvalid
	}
	return dispatcher.replayPolicy
}

// Requests returns consumed Dispatch requests in actual call order, including
// requests that mismatch or exceed the script.
func (dispatcher *ScriptedDispatcher) Requests() []agent.EffectRequest {
	if dispatcher == nil {
		return nil
	}
	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	return slices.Clone(dispatcher.requests)
}

// Remaining reports how many scripted calls have not been consumed.
func (dispatcher *ScriptedDispatcher) Remaining() int {
	if dispatcher == nil {
		return 0
	}
	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	return len(dispatcher.steps) - dispatcher.next
}
