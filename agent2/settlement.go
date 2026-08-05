package agent2

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

var ErrInvalidSettlement = errors.New("agent: invalid effect settlement")

// SettlementStatus records whether an Effect definitely succeeded, definitely
// failed, or has an unknown external result. Unknown never implies safe retry.
type SettlementStatus uint8

const (
	SettlementStatusInvalid SettlementStatus = iota
	SettlementStatusSucceeded
	SettlementStatusFailed
	SettlementStatusUnknown
)

func (status SettlementStatus) String() string {
	switch status {
	case SettlementStatusSucceeded:
		return "succeeded"
	case SettlementStatusFailed:
		return "failed"
	case SettlementStatusUnknown:
		return "unknown"
	default:
		return "invalid"
	}
}

func parseSettlementStatus(value string) (SettlementStatus, error) {
	switch value {
	case "succeeded":
		return SettlementStatusSucceeded, nil
	case "failed":
		return SettlementStatusFailed, nil
	case "unknown":
		return SettlementStatusUnknown, nil
	default:
		return SettlementStatusInvalid, fmt.Errorf("%w: unknown status %q", ErrInvalidSettlement, value)
	}
}

// Settlement is the immutable final fact for one EffectID. Payload is owned by
// the Effect target and becomes opaque Signal data for the next Step. The
// Engine uses Status only to preserve definite versus unknown execution facts.
type Settlement struct {
	effectID EffectID
	status   SettlementStatus
	payload  json.RawMessage
}

// NewSettlement validates and freezes one Effect result.
func NewSettlement(effectID EffectID, status SettlementStatus, payload json.RawMessage) (Settlement, error) {
	if !effectID.Valid() {
		return Settlement{}, fmt.Errorf("%w: effect ID: %w", ErrInvalidSettlement, ErrInvalidIdentity)
	}
	if status < SettlementStatusSucceeded || status > SettlementStatusUnknown {
		return Settlement{}, fmt.Errorf("%w: status is required", ErrInvalidSettlement)
	}
	normalized, err := normalizeJSON(payload, maxWireBytes)
	if err != nil {
		return Settlement{}, fmt.Errorf("%w: payload: %w", ErrInvalidSettlement, err)
	}
	return Settlement{effectID: effectID, status: status, payload: normalized}, nil
}

// EffectID returns the Effect this result settles.
func (s Settlement) EffectID() EffectID { return s.effectID }

// Status returns whether the external result is definite or unknown.
func (s Settlement) Status() SettlementStatus { return s.status }

// Payload returns an independently owned owner-defined result.
func (s Settlement) Payload() json.RawMessage { return bytes.Clone(s.payload) }

// Valid reports whether the Settlement has a complete immutable envelope.
func (s Settlement) Valid() bool {
	return s.effectID.Valid() && s.status >= SettlementStatusSucceeded &&
		s.status <= SettlementStatusUnknown && len(s.payload) > 0
}

func (s Settlement) MarshalJSON() ([]byte, error) {
	if !s.Valid() {
		return nil, ErrInvalidSettlement
	}
	return json.Marshal(settlementWire{EffectID: s.effectID, Status: s.status.String(), Payload: s.payload})
}

func (s *Settlement) UnmarshalJSON(data []byte) error {
	if s == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidSettlement)
	}
	var wire settlementWire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return fmt.Errorf("%w: decode: %w", ErrInvalidSettlement, err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidSettlement, err)
	}
	status, err := parseSettlementStatus(wire.Status)
	if err != nil {
		return err
	}
	value, err := NewSettlement(wire.EffectID, status, wire.Payload)
	if err != nil {
		return err
	}
	*s = value
	return nil
}

type settlementWire struct {
	EffectID EffectID        `json:"effect_id"`
	Status   string          `json:"status"`
	Payload  json.RawMessage `json:"payload"`
}
