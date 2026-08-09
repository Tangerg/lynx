package interaction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	agent "github.com/Tangerg/lynx/agent"
)

const maxInputProtocolBytes = 1 << 20

var (
	// ErrInvalidToolInputRequest reports malformed prompt, response schema, or
	// continuation state supplied by a Tool.
	ErrInvalidToolInputRequest = errors.New("interaction: invalid tool input request")
	// ErrToolInputRequired is matched by errors.Is when a Tool requests external
	// input before it can produce a ToolResult.
	ErrToolInputRequired = errors.New("interaction: tool input required")
)

// ToolInputRequest is an immutable Tool request for external input. Prompt is an
// owner-defined JSON value for a consumer, ResponseSchema is authoritative,
// and ContinuationState is returned only to the same Tool when input arrives.
// It contains no Process identity or WaitID; Engine owns those identities.
type ToolInputRequest struct {
	prompt            json.RawMessage
	responseSchema    json.RawMessage
	continuationState json.RawMessage
}

// NewToolInputRequest validates and freezes a Tool input request.
func NewToolInputRequest(
	prompt json.RawMessage,
	responseSchema json.RawMessage,
	continuationState json.RawMessage,
) (ToolInputRequest, error) {
	prompt, err := canonicalJSON(prompt)
	if err != nil {
		return ToolInputRequest{}, fmt.Errorf("%w: prompt: %w", ErrInvalidToolInputRequest, err)
	}
	responseSchema, err = canonicalJSON(responseSchema)
	if err != nil {
		return ToolInputRequest{}, fmt.Errorf("%w: response schema: %w", ErrInvalidToolInputRequest, err)
	}
	if _, err := agent.ParseSchema(responseSchema); err != nil {
		return ToolInputRequest{}, fmt.Errorf("%w: response schema: %w", ErrInvalidToolInputRequest, err)
	}
	continuationState, err = canonicalJSON(continuationState)
	if err != nil {
		return ToolInputRequest{}, fmt.Errorf("%w: continuation state: %w", ErrInvalidToolInputRequest, err)
	}
	return ToolInputRequest{
		prompt: prompt, responseSchema: responseSchema, continuationState: continuationState,
	}, nil
}

// Prompt returns an independently owned consumer-facing JSON value.
func (request ToolInputRequest) Prompt() json.RawMessage { return bytes.Clone(request.prompt) }

// ResponseSchema returns the authoritative JSON Schema for an answer.
func (request ToolInputRequest) ResponseSchema() json.RawMessage {
	return bytes.Clone(request.responseSchema)
}

// ContinuationState returns opaque state owned by the requesting Tool.
func (request ToolInputRequest) ContinuationState() json.RawMessage {
	return bytes.Clone(request.continuationState)
}

// Valid reports whether the request was constructed through NewToolInputRequest.
func (request ToolInputRequest) Valid() bool {
	return len(request.prompt) > 0 && len(request.responseSchema) > 0 && len(request.continuationState) > 0
}

func (request ToolInputRequest) validateResponse(response json.RawMessage) (json.RawMessage, error) {
	if !request.Valid() {
		return nil, ErrInvalidToolInputRequest
	}
	response, err := canonicalJSON(response)
	if err != nil {
		return nil, fmt.Errorf("%w: response: %w", ErrInvalidToolInputRequest, err)
	}
	schema, err := agent.ParseSchema(request.responseSchema)
	if err != nil {
		return nil, fmt.Errorf("%w: response schema: %w", ErrInvalidToolInputRequest, err)
	}
	input, err := agent.ParseInput(response)
	if err != nil {
		return nil, fmt.Errorf("%w: response: %w", ErrInvalidToolInputRequest, err)
	}
	if err := schema.ValidateInput(input); err != nil {
		return nil, fmt.Errorf("%w: response: %w", ErrInvalidToolInputRequest, err)
	}
	return response, nil
}

// ToolInputRequiredError carries one validated, snapshot-safe ToolInputRequest across
// a Tool boundary. It is control flow, not a failed ToolResult.
type ToolInputRequiredError struct {
	request ToolInputRequest
}

// RequireToolInput validates the request and returns an error matching
// ErrToolInputRequired. A Tool returns this before external side effects, or after
// storing enough ContinuationState to prove safe re-entry.
func RequireToolInput(
	prompt json.RawMessage,
	responseSchema json.RawMessage,
	continuationState json.RawMessage,
) error {
	request, err := NewToolInputRequest(prompt, responseSchema, continuationState)
	if err != nil {
		return err
	}
	return &ToolInputRequiredError{request: request}
}

// Error returns the stable Tool input control-flow sentinel text.
func (required *ToolInputRequiredError) Error() string {
	if required == nil {
		return ErrToolInputRequired.Error()
	}
	return ErrToolInputRequired.Error()
}

// Unwrap supports errors.Is(err, ErrToolInputRequired).
func (*ToolInputRequiredError) Unwrap() error { return ErrToolInputRequired }

func (required *ToolInputRequiredError) inputRequest() (ToolInputRequest, bool) {
	if required == nil || !required.request.Valid() {
		return ToolInputRequest{}, false
	}
	return required.request, true
}

// ToolInputContinuation is the immutable state and validated external response
// attached only while re-entering the Tool that requested input.
type ToolInputContinuation struct {
	state    json.RawMessage
	response json.RawMessage
}

// State returns the Tool-owned continuation state captured at suspension.
func (continuation ToolInputContinuation) State() json.RawMessage {
	return bytes.Clone(continuation.state)
}

// Response returns the schema-validated external input.
func (continuation ToolInputContinuation) Response() json.RawMessage {
	return bytes.Clone(continuation.response)
}

type toolContinuationContextKey struct{}

func withToolInputContinuation(ctx context.Context, continuation ToolInputContinuation) context.Context {
	return context.WithValue(ctx, toolContinuationContextKey{}, continuation)
}

// ToolInputContinuationFromContext returns continuation data only for the active
// resumed Tool call. Ordinary first attempts return false.
func ToolInputContinuationFromContext(ctx context.Context) (ToolInputContinuation, bool) {
	if ctx == nil {
		return ToolInputContinuation{}, false
	}
	continuation, ok := ctx.Value(toolContinuationContextKey{}).(ToolInputContinuation)
	if !ok || len(continuation.state) == 0 || len(continuation.response) == 0 {
		return ToolInputContinuation{}, false
	}
	return ToolInputContinuation{
		state: bytes.Clone(continuation.state), response: bytes.Clone(continuation.response),
	}, true
}

// NewToolInputResponseSignal constructs one WaitID-addressed, deduplicated answer
// for an Interaction input request. The active Execution validates response
// against the Tool-supplied schema before it advances the mailbox cursor.
func NewToolInputResponseSignal(
	id agent.SignalID,
	waitID agent.WaitID,
	response json.RawMessage,
) (agent.SignalRequest, error) {
	if !waitID.Valid() {
		return agent.SignalRequest{}, fmt.Errorf("%w: WaitID is required", ErrInvalidToolInputRequest)
	}
	response, err := canonicalJSON(response)
	if err != nil {
		return agent.SignalRequest{}, fmt.Errorf("%w: response: %w", ErrInvalidToolInputRequest, err)
	}
	payload, err := encodeProtocol(signalEnvelope{
		SchemaVersion: protocolSchemaVersion,
		Operation:     operationInputResponse,
		InputResponse: response,
	})
	if err != nil {
		return agent.SignalRequest{}, err
	}
	return agent.NewSignalRequest(id, waitID, payload)
}

func canonicalJSON(data json.RawMessage) (json.RawMessage, error) {
	if len(data) == 0 || len(data) > maxInputProtocolBytes {
		return nil, fmt.Errorf("JSON value must contain at most %d bytes", maxInputProtocolBytes)
	}
	var value any
	if err := decodeStrict(data, &value); err != nil {
		return nil, err
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return canonical, nil
}
