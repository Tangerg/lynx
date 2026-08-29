package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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

func NewSignalRequest(id SignalID, waitID WaitID, payload json.RawMessage) (SignalRequest, error) {
	if !id.Valid() {
		return SignalRequest{}, fmt.Errorf("%w: signal ID: %w", ErrInvalidSignalRequest, ErrInvalidIdentity)
	}
	normalized, err := wireJSON.normalize(payload, maxWireBytes)
	if err != nil {
		return SignalRequest{}, fmt.Errorf("%w: payload: %w", ErrInvalidSignalRequest, err)
	}
	return SignalRequest{id: id, waitID: waitID, payload: normalized}, nil
}

// ID returns the stable delivery and deduplication identity.
func (s SignalRequest) ID() SignalID { return s.id }

// WaitID returns the addressed wait and true, or a zero WaitID and false.
func (s SignalRequest) WaitID() (WaitID, bool) { return s.waitID, s.waitID.Valid() }

// Payload returns an independently owned Strategy-defined value.
func (s SignalRequest) Payload() json.RawMessage { return bytes.Clone(s.payload) }

func (s SignalRequest) Valid() bool { return s.id.Valid() && len(s.payload) > 0 }

func (s SignalRequest) signal() (Signal, error) {
	if !s.Valid() {
		return Signal{}, ErrInvalidSignalRequest
	}
	return newSignal(s.id, s.waitID, s.payload)
}
