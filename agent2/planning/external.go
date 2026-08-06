package planning

import (
	"context"
	"errors"
	"fmt"

	agent "github.com/Tangerg/lynx/agent2"
)

// ObservationRequest is one side-effect-free request for the current complete
// WorldState. Input is the original Planning Process input; EffectID is stable
// for the prepared attempt.
type ObservationRequest struct {
	EffectID agent.EffectID
	Input    agent.Input
}

// ActionRequest is one external dispatcher Action invocation selected against
// an observed WorldState. Input and WorldState are immutable values.
type ActionRequest struct {
	EffectID    agent.EffectID
	Input       agent.Input
	ActionName  string
	Description string
	WorldState  WorldState
}

// Observer produces one complete WorldState without externally visible side
// effects. A returned error is a definite observation failure and terminates
// Planning; an Observer must not use error to report an unknown side effect.
type Observer interface {
	Observe(context.Context, ObservationRequest) (WorldState, error)
}

// ObserverFunc adapts a function to Observer.
type ObserverFunc func(context.Context, ObservationRequest) (WorldState, error)

// Observe calls function with request.
func (function ObserverFunc) Observe(
	ctx context.Context,
	request ObservationRequest,
) (WorldState, error) {
	return function(ctx, request)
}

// ActionExecutor performs one dispatcher-bound Action. A valid ActionResult is
// a definite success or failure. A non-nil error means the external outcome is
// unknown, so Dispatcher returns an unknown Effect settlement and never retries
// it implicitly.
type ActionExecutor interface {
	Execute(context.Context, ActionRequest) (ActionResult, error)
}

// ActionExecutorFunc adapts a function to ActionExecutor.
type ActionExecutorFunc func(context.Context, ActionRequest) (ActionResult, error)

// Execute calls function with request.
func (function ActionExecutorFunc) Execute(
	ctx context.Context,
	request ActionRequest,
) (ActionResult, error) {
	return function(ctx, request)
}

// ActionResult is the definite external result reported by an ActionExecutor.
// Its zero value is invalid.
type ActionResult struct {
	succeeded  bool
	diagnostic string
	valid      bool
}

// ActionSucceeded returns a definite successful Action result.
func ActionSucceeded() ActionResult { return ActionResult{succeeded: true, valid: true} }

// ActionFailed constructs a definite failed Action result with a bounded
// diagnostic suitable for a portable Planning attempt record.
func ActionFailed(diagnostic string) (ActionResult, error) {
	if !validDiagnostic(diagnostic) {
		return ActionResult{}, errors.New("planning: Action failure diagnostic must be non-empty, trimmed, and bounded")
	}
	return ActionResult{diagnostic: diagnostic, valid: true}, nil
}

// Succeeded reports whether the definite Action result succeeded.
func (result ActionResult) Succeeded() bool { return result.valid && result.succeeded }

// Diagnostic returns the definite failure explanation, or an empty string on
// success.
func (result ActionResult) Diagnostic() string { return result.diagnostic }

// Valid reports whether result was constructed as a definite success or failure.
func (result ActionResult) Valid() bool {
	return result.valid && (result.succeeded && result.diagnostic == "" ||
		!result.succeeded && validDiagnostic(result.diagnostic))
}

// NewActionSettlement constructs the definite settlement used to resolve an
// Action Effect whose first dispatch attempt had an unknown external outcome.
// The caller must use the original EffectID; Engine rejects identities that do
// not currently require resolution.
func NewActionSettlement(effectID agent.EffectID, result ActionResult) (agent.Settlement, error) {
	if !effectID.Valid() || !result.Valid() {
		return agent.Settlement{}, ErrInvalidProtocol
	}
	payload, err := actionSignal(result)
	if err != nil {
		return agent.Settlement{}, err
	}
	status := agent.SettlementStatusSucceeded
	if !result.Succeeded() {
		status = agent.SettlementStatusFailed
	}
	return agent.NewSettlement(effectID, status, payload)
}

func validateObservationRequest(request ObservationRequest) error {
	if !request.EffectID.Valid() || !request.Input.Valid() {
		return errors.New("planning: invalid observation request")
	}
	return nil
}

func validateActionRequest(request ActionRequest) error {
	if !request.EffectID.Valid() || !request.Input.Valid() || !validName(request.ActionName) ||
		!validDescription(request.Description) || !request.WorldState.Valid() {
		return fmt.Errorf("planning: invalid Action request for %q", request.ActionName)
	}
	return nil
}
