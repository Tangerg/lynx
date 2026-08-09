package interaction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	agent "github.com/Tangerg/lynx/agent"
)

// ErrInvalidPendingToolInput reports a Waiting Process whose opaque
// Interaction state and Engine wait identity disagree or cannot be decoded.
var ErrInvalidPendingToolInput = errors.New("interaction: invalid pending tool input")

// PendingToolInput is the consumer-facing view of one current Tool input wait.
// It deliberately excludes Tool continuation state and all application UI,
// persistence, approval, or actor concepts.
type PendingToolInput struct {
	waitID         agent.WaitID
	prompt         json.RawMessage
	responseSchema json.RawMessage
}

// WaitID returns the Engine-minted identity required to address the response.
func (pending PendingToolInput) WaitID() agent.WaitID { return pending.waitID }

// Prompt returns an independently owned Tool-defined JSON prompt.
func (pending PendingToolInput) Prompt() json.RawMessage { return bytes.Clone(pending.prompt) }

// ResponseSchema returns the authoritative JSON Schema for a response.
func (pending PendingToolInput) ResponseSchema() json.RawMessage {
	return bytes.Clone(pending.responseSchema)
}

// Valid reports whether the value identifies one complete current wait.
func (pending PendingToolInput) Valid() bool {
	return pending.waitID.Valid() && len(pending.prompt) > 0 && len(pending.responseSchema) > 0
}

// ResponseSignal validates response locally against ResponseSchema and returns
// one WaitID-addressed SignalRequest with caller-supplied deduplication ID.
func (pending PendingToolInput) ResponseSignal(
	id agent.SignalID,
	response json.RawMessage,
) (agent.SignalRequest, error) {
	if !pending.Valid() {
		return agent.SignalRequest{}, ErrInvalidPendingToolInput
	}
	request, err := NewToolInputRequest(pending.prompt, pending.responseSchema, json.RawMessage("null"))
	if err != nil {
		return agent.SignalRequest{}, fmt.Errorf("%w: %w", ErrInvalidPendingToolInput, err)
	}
	response, err = request.validateResponse(response)
	if err != nil {
		return agent.SignalRequest{}, err
	}
	return NewToolInputResponseSignal(id, pending.waitID, response)
}

// PendingToolInputFromProcess captures process and interprets its committed
// state only when it is an Interaction currently waiting for Tool input.
func PendingToolInputFromProcess(
	ctx context.Context,
	process *agent.Process,
) (PendingToolInput, bool, error) {
	if process == nil {
		return PendingToolInput{}, false, ErrInvalidPendingToolInput
	}
	snapshot, err := process.Capture(ctx)
	if err != nil {
		return PendingToolInput{}, false, err
	}
	return PendingToolInputFromSnapshot(snapshot)
}

// PendingToolInputFromSnapshot interprets only Interaction-owned state. A
// valid non-Waiting or non-Interaction snapshot returns found=false.
func PendingToolInputFromSnapshot(snapshot agent.Snapshot) (PendingToolInput, bool, error) {
	if !snapshot.Valid() {
		return PendingToolInput{}, false, ErrInvalidPendingToolInput
	}
	if snapshot.Status() != agent.StatusWaiting {
		return PendingToolInput{}, false, nil
	}
	stateEnvelope := snapshot.CommittedExecutionState()
	if stateEnvelope.Kind() != executionStateKind || stateEnvelope.SchemaVersion() != executionStateSchemaVersion {
		return PendingToolInput{}, false, nil
	}
	var state executionState
	if err := decodeStrict(stateEnvelope.Payload(), &state); err != nil {
		return PendingToolInput{}, false, fmt.Errorf("%w: decode state: %w", ErrInvalidPendingToolInput, err)
	}
	outerWaitID, ok := snapshot.WaitID()
	switch state.Phase {
	case phaseWaitingDelegates:
		if _, err := state.activeDelegateCalls(); err != nil {
			return PendingToolInput{}, false, fmt.Errorf("%w: %w", ErrInvalidPendingToolInput, err)
		}
		if state.WaitID == nil || !ok || outerWaitID != *state.WaitID {
			return PendingToolInput{}, false, ErrInvalidPendingToolInput
		}
		return PendingToolInput{}, false, nil
	case phaseWaitingInput:
	default:
		return PendingToolInput{}, false, ErrInvalidPendingToolInput
	}
	if err := state.validatePendingToolInput(); err != nil {
		return PendingToolInput{}, false, fmt.Errorf("%w: %w", ErrInvalidPendingToolInput, err)
	}
	if state.WaitID == nil || !ok || outerWaitID != *state.WaitID || state.ToolCheckpoint == nil {
		return PendingToolInput{}, false, ErrInvalidPendingToolInput
	}
	request, err := state.ToolCheckpoint.InputRequest.inputRequest()
	if err != nil {
		return PendingToolInput{}, false, fmt.Errorf("%w: %w", ErrInvalidPendingToolInput, err)
	}
	return PendingToolInput{
		waitID: outerWaitID, prompt: request.Prompt(), responseSchema: request.ResponseSchema(),
	}, true, nil
}
