package agent2

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var ErrInvalidSignalRequest = errors.New("agent: invalid signal request")

// SignalRequest is an immutable request to deliver Strategy-owned input to a
// Process. ID supplies caller-stable deduplication. WaitID is zero for ordinary
// next-boundary input and Engine-minted for an external wait answer.
type SignalRequest struct {
	id      SignalID
	waitID  WaitID
	payload json.RawMessage
}

// NewSignalRequest validates and freezes one delivery request. Parsing a WaitID
// does not make it valid for a Process; the Engine checks its wait mapping.
func NewSignalRequest(id SignalID, waitID WaitID, payload json.RawMessage) (SignalRequest, error) {
	if !id.Valid() {
		return SignalRequest{}, fmt.Errorf("%w: signal ID: %w", ErrInvalidSignalRequest, ErrInvalidIdentity)
	}
	normalized, err := normalizeJSON(payload, maxWireBytes)
	if err != nil {
		return SignalRequest{}, fmt.Errorf("%w: payload: %w", ErrInvalidSignalRequest, err)
	}
	return SignalRequest{id: id, waitID: waitID, payload: normalized}, nil
}

// ID returns the stable delivery and deduplication identity.
func (request SignalRequest) ID() SignalID { return request.id }

// WaitID returns the addressed wait and true, or a zero WaitID and false.
func (request SignalRequest) WaitID() (WaitID, bool) { return request.waitID, request.waitID.Valid() }

// Payload returns an independently owned Strategy-defined value.
func (request SignalRequest) Payload() json.RawMessage { return bytes.Clone(request.payload) }

// Valid reports whether the request has a complete immutable envelope.
func (request SignalRequest) Valid() bool { return request.id.Valid() && len(request.payload) > 0 }

func (request SignalRequest) signal(receivedAt time.Time) (Signal, error) {
	if !request.Valid() {
		return Signal{}, ErrInvalidSignalRequest
	}
	return newSignal(request.id, request.waitID, receivedAt, request.payload)
}
