package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrInvalidSignal reports a malformed accepted execution input.
var ErrInvalidSignal = errors.New("agent: invalid signal")

// Signal is the immutable input envelope delivered by the Engine to an
// Execution. Dispatcher and ordinary wait payloads belong exclusively to the
// Strategy; Framework composition payloads are decoded only through their
// public typed helpers. SignalID identifies delivery, while an optional WaitID
// identifies the Engine-created wait target.
type Signal struct {
	id         SignalID
	waitID     WaitID
	receivedAt time.Time
	payload    json.RawMessage
}

func newSignal(id SignalID, waitID WaitID, receivedAt time.Time, payload json.RawMessage) (Signal, error) {
	if !id.Valid() {
		return Signal{}, fmt.Errorf("%w: %w", ErrInvalidSignal, ErrInvalidIdentity)
	}
	if receivedAt.IsZero() {
		return Signal{}, fmt.Errorf("%w: received time is required", ErrInvalidSignal)
	}
	normalized, err := wireJSON.normalize(payload, maxWireBytes)
	if err != nil {
		return Signal{}, fmt.Errorf("%w: payload: %w", ErrInvalidSignal, err)
	}
	return Signal{
		id:         id,
		waitID:     waitID,
		receivedAt: receivedAt.Round(0).UTC(),
		payload:    normalized,
	}, nil
}

// ID returns the stable delivery and deduplication identity.
func (s Signal) ID() SignalID { return s.id }

// WaitID returns the addressed wait and true, or a zero WaitID and false for a
// Signal queued at the next Strategy-safe boundary.
func (s Signal) WaitID() (WaitID, bool) { return s.waitID, s.waitID.Valid() }

// ReceivedAt returns when the Engine accepted the Signal.
func (s Signal) ReceivedAt() time.Time { return s.receivedAt }

// Payload returns an independently owned copy. Strategy-owned payloads are
// interpreted only by their Strategy; Framework-owned payloads should be read
// through the corresponding typed parser rather than decoded ad hoc.
func (s Signal) Payload() json.RawMessage { return bytes.Clone(s.payload) }

// Valid reports whether the Signal has a complete immutable envelope.
func (s Signal) Valid() bool {
	return s.id.Valid() && !s.receivedAt.IsZero() && len(s.payload) > 0
}

// MarshalJSON returns the validated immutable Signal envelope.
func (s Signal) MarshalJSON() ([]byte, error) {
	if !s.Valid() {
		return nil, ErrInvalidSignal
	}
	wire := signalWire{
		ID:         s.id,
		ReceivedAt: s.receivedAt,
		Payload:    s.payload,
	}
	if s.waitID.Valid() {
		wire.WaitID = &s.waitID
	}
	return json.Marshal(wire)
}

// UnmarshalJSON replaces s with a strictly decoded Signal.
func (s *Signal) UnmarshalJSON(data []byte) error {
	if s == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidSignal)
	}
	var wire signalWire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return fmt.Errorf("%w: decode: %w", ErrInvalidSignal, err)
	}
	if err := wireJSON.requireEOF(decoder); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidSignal, err)
	}
	var waitID WaitID
	if wire.WaitID != nil {
		waitID = *wire.WaitID
	}
	value, err := newSignal(wire.ID, waitID, wire.ReceivedAt, wire.Payload)
	if err != nil {
		return err
	}
	*s = value
	return nil
}

type signalWire struct {
	ID         SignalID        `json:"id"`
	WaitID     *WaitID         `json:"wait_id,omitempty"`
	ReceivedAt time.Time       `json:"received_at"`
	Payload    json.RawMessage `json:"payload"`
}
