package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// ErrInvalidSettlement reports a malformed or misaddressed Effect outcome.
var ErrInvalidSettlement = errors.New("agent: invalid effect settlement")

// SettlementStatus records whether an Effect definitely succeeded, definitely
// failed, or has an unknown external result. Unknown never implies safe retry.
type SettlementStatus string

const (
	// SettlementStatusInvalid is the invalid zero value.
	SettlementStatusInvalid SettlementStatus = ""
	// SettlementStatusSucceeded records a definite successful outcome.
	SettlementStatusSucceeded SettlementStatus = "succeeded"
	// SettlementStatusFailed records a definite failed outcome.
	SettlementStatusFailed SettlementStatus = "failed"
	// SettlementStatusUnknown records an externally indeterminate outcome.
	SettlementStatusUnknown SettlementStatus = "unknown"
)

// Valid reports whether s is a definite or indeterminate settlement fact.
func (s SettlementStatus) Valid() bool {
	switch s {
	case SettlementStatusSucceeded, SettlementStatusFailed, SettlementStatusUnknown:
		return true
	default:
		return false
	}
}

// String returns the stable settlement-status name.
func (s SettlementStatus) String() string {
	if !s.Valid() {
		return "invalid"
	}
	return string(s)
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
	if !status.Valid() {
		return Settlement{}, fmt.Errorf("%w: status is required", ErrInvalidSettlement)
	}
	normalized, err := wireJSON.normalize(payload, maxWireBytes)
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
	return s.effectID.Valid() && s.status.Valid() && len(s.payload) > 0
}

// MarshalJSON returns the validated immutable Effect settlement.
func (s Settlement) MarshalJSON() ([]byte, error) {
	if !s.Valid() {
		return nil, ErrInvalidSettlement
	}
	return json.Marshal(settlementWire{EffectID: s.effectID, Status: s.status, Payload: s.payload})
}

// UnmarshalJSON replaces s with a strictly decoded Settlement.
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
	if err := wireJSON.requireEOF(decoder); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidSettlement, err)
	}
	value, err := NewSettlement(wire.EffectID, wire.Status, wire.Payload)
	if err != nil {
		return err
	}
	*s = value
	return nil
}

type settlementWire struct {
	EffectID EffectID         `json:"effect_id"`
	Status   SettlementStatus `json:"status"`
	Payload  json.RawMessage  `json:"payload"`
}
